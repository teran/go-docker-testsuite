package docker

import (
	"fmt"
	"strconv"

	log "github.com/sirupsen/logrus"
)

// Environment represents the container environment passed
// into runtime
type Environment map[string]string

// NewEnvironment creates new Environment instance
func NewEnvironment() Environment {
	return Environment{}
}

// StringVar sets string var to the environment
func (e Environment) StringVar(name, value string) Environment {
	e[name] = value
	return e
}

// LogLevelVar sets logrus.Level var to the environment
func (e Environment) LogLevelVar(name string, l log.Level) Environment {
	e[name] = l.String()
	return e
}

// Int64Var sets int64 var to the envorinment
func (e Environment) Int64Var(name string, value int64) Environment {
	e[name] = strconv.FormatInt(value, 10)
	return e
}

// IntVar sets int var to the environment
func (e Environment) IntVar(name string, value int) Environment {
	return e.Int64Var(name, int64(value))
}

// Int32Var sets int32 var to the environment
func (e Environment) Int32Var(name string, value int32) Environment {
	return e.Int64Var(name, int64(value))
}

// Int16Var sets int16 var to the environment
func (e Environment) Int16Var(name string, value int16) Environment {
	return e.Int64Var(name, int64(value))
}

// Int8Var sets int8 var to the environment
func (e Environment) Int8Var(name string, value int8) Environment {
	return e.Int64Var(name, int64(value))
}

// Uint64Var sets uint64 var to the environment
func (e Environment) Uint64Var(name string, value uint64) Environment {
	e[name] = strconv.FormatUint(value, 10)
	return e
}

// UintVar sets uint var to the environent
func (e Environment) UintVar(name string, value uint) Environment {
	return e.Uint64Var(name, uint64(value))
}

// Uint32Var sets uint32 var to the environment
func (e Environment) Uint32Var(name string, value uint32) Environment {
	return e.Uint64Var(name, uint64(value))
}

// Uint16Var sets uint16 var to the environment
func (e Environment) Uint16Var(name string, value uint16) Environment {
	return e.Uint64Var(name, uint64(value))
}

// Uint8Var sets uint8 var to the environment
func (e Environment) Uint8Var(name string, value uint8) Environment {
	return e.Uint64Var(name, uint64(value))
}

// BoolVar sets bool var to the environment
func (e Environment) BoolVar(name string, value bool) Environment {
	e[name] = strconv.FormatBool(value)
	return e
}

func (e Environment) list() (es []string) {
	for k, v := range e {
		es = append(es, fmt.Sprintf("%s=%s", k, v))
	}
	return es
}
