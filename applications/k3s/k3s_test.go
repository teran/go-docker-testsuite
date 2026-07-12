package k3s

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"


)

const (
	testTimeout    = 5 * time.Minute
	cleanupTimeout = 1 * time.Minute
)

func init() {
	log.SetLevel(log.TraceLevel)
}

type k3sTestSuite struct {
	suite.Suite

	ctx        context.Context
	cancelFunc context.CancelFunc
	app        K3s
	clientset  *kubernetes.Clientset
	dockerCli  *client.Client
}

func (s *k3sTestSuite) SetupSuite() {
	var err error
	s.app, err = New(s.ctx)
	s.Require().NoError(err)
	s.Require().NotNil(s.app)

	s.clientset, err = s.app.Clientset(s.ctx)
	s.Require().NoError(err)
	s.Require().NotNil(s.clientset)

	s.dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	s.Require().NoError(err)
}

func (s *k3sTestSuite) TearDownSuite() {
	ctx, cancel := context.WithTimeout(s.ctx, cleanupTimeout)
	defer cancel()
	defer s.cancelFunc()

	if s.dockerCli != nil {
		_ = s.dockerCli.Close()
	}

	err := s.app.Close(ctx)
	s.Require().NoError(err)
}

func TestK3sTestSuite(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	defer cancel()

	suite.Run(t, &k3sTestSuite{
		ctx:        ctx,
		cancelFunc: cancel,
	})
}

func (s *k3sTestSuite) TestClientset() {
	r := s.Require()

	// Verify clientset is usable by listing nodes
	nodes, err := s.clientset.CoreV1().Nodes().List(s.ctx, metav1.ListOptions{})
	r.NoError(err)
	r.Len(nodes.Items, 1)

	s.T().Logf("k3s node: %s", nodes.Items[0].Name)

	// Verify server version
	sv, err := s.clientset.Discovery().ServerVersion()
	r.NoError(err)
	r.NotEmpty(sv.GitVersion)
	s.T().Logf("k3s version: %s", sv.GitVersion)
}

func (s *k3sTestSuite) TestKubeconfigPath() {
	r := s.Require()

	kubeconfigPath, err := s.app.KubeconfigPath()
	r.NoError(err)
	r.NotEmpty(kubeconfigPath)
	s.T().Logf("kubeconfig path: %s", kubeconfigPath)

	// Verify file exists and is non-empty
	info, err := os.Stat(kubeconfigPath)
	r.NoError(err)
	r.Greater(info.Size(), int64(0))

	// Read and verify it is valid YAML
	data, err := os.ReadFile(kubeconfigPath)
	r.NoError(err)

	var out interface{}
	err = yaml.Unmarshal(data, &out)
	r.NoError(err, "kubeconfig must be valid YAML")
}

func (s *k3sTestSuite) TestLoadBalancerService() {
	r := s.Require()

	echoName := "echo-server"
	echoPort := int32(80)

	// --- Create Deployment ---
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: echoName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": echoName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": echoName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  echoName,
							Image: "index.docker.io/library/nginx:alpine",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: echoPort,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("32Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := s.clientset.AppsV1().Deployments("default").Create(s.ctx, deployment, metav1.CreateOptions{})
	r.NoError(err)

	// Cleanup deployment
	defer func() {
		_ = s.clientset.AppsV1().Deployments("default").Delete(
			s.ctx, echoName, metav1.DeleteOptions{},
		)
	}()

	// --- Create LoadBalancer Service ---
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: echoName,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{
					Port:       echoPort,
					TargetPort: intstr.FromInt(int(echoPort)),
				},
			},
			Selector: map[string]string{
				"app": echoName,
			},
		},
	}

	_, err = s.clientset.CoreV1().Services("default").Create(s.ctx, service, metav1.CreateOptions{})
	r.NoError(err)

	// Cleanup service
	defer func() {
		_ = s.clientset.CoreV1().Services("default").Delete(
			s.ctx, echoName, metav1.DeleteOptions{},
		)
	}()

	// --- Wait for Deployment to be ready ---
	err = wait.PollUntilContextTimeout(s.ctx, 1*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		dep, err := s.clientset.AppsV1().Deployments("default").Get(ctx, echoName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return dep.Status.AvailableReplicas == 1, nil
	})
	r.NoError(err, "deployment should have 1 available replica")

	// --- Wait for LoadBalancer ingress to be assigned by klipper ---
	var lbIngressHost string
	err = wait.PollUntilContextTimeout(s.ctx, 1*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		svc, err := s.clientset.CoreV1().Services("default").Get(ctx, echoName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			ing := svc.Status.LoadBalancer.Ingress[0]
			if ing.IP != "" {
				lbIngressHost = ing.IP
				return true, nil
			}
			if ing.Hostname != "" {
				lbIngressHost = ing.Hostname
				return true, nil
			}
		}
		return false, nil
	})
	r.NoError(err, "LoadBalancer ingress should be assigned by klipper")
	r.NotEmpty(lbIngressHost)
	s.T().Logf("LoadBalancer ingress: %s", lbIngressHost)

	// --- Verify NodePort is assigned in the valid range ---
	svc, err := s.clientset.CoreV1().Services("default").Get(s.ctx, echoName, metav1.GetOptions{})
	r.NoError(err)
	r.Len(svc.Spec.Ports, 1)
	nodePort := svc.Spec.Ports[0].NodePort
	r.GreaterOrEqual(nodePort, int32(30000))
	r.LessOrEqual(nodePort, int32(32767))
	s.T().Logf("Service NodePort: %d", nodePort)

	// --- Verify connectivity via exec inside the k3s container ---
	// On Docker Desktop for macOS, bridge IPs are not directly reachable
	// from the host. Instead, we verify by curling the service from inside
	// the k3s container using Docker exec.
	svcURL := fmt.Sprintf("http://%s:%d/", lbIngressHost, echoPort)
	output, err := execInContainer(s.ctx, s.dockerCli, s.app.ID(),
		"wget", "-qO-", "--timeout=10", svcURL)
	r.NoError(err, "should be able to reach the service from inside the container")

	s.T().Logf("Service response (from inside container): %s", strings.TrimSpace(string(output)))
	s.T().Log("Successfully verified HTTP communication through LoadBalancer service")
}


