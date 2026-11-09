package utils

import (
	"fmt"
	"os"
)

func EnvOr(envName string) string {
	envVarValue := os.Getenv(envName)
	if envVarValue == "" {
		panic(fmt.Errorf("Env variable %s is not set\n", envName))
	}
	return envVarValue
}
