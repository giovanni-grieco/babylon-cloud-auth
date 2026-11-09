package clients

import (
	"bufio"
	"control-plane-svc/internals/utils"
	"encoding/json"
	"os"
)

type RealmsConfigMap map[string]RealmConfig

type RealmsConfig []RealmConfig

type RealmConfig struct {
	Id               string `json:"id"`
	Name             string `json:"name"`
	BaseURL          string `json:"baseURL"`
	Realm            string `json:"realm"`
	ClientID         string `json:"clientID"`
	SecretEnvVarName string `json:"secretEnv"`
	SecretEnv        string
}

func LoadRealmConfig(cpRealmConfigFilePath string) RealmsConfigMap {

	file, err := os.Open(cpRealmConfigFilePath)
	if err != nil {
		panic(err)
	}

	var realmsConfig RealmsConfig

	err = json.NewDecoder(bufio.NewReader(file)).Decode(&realmsConfig)
	if err != nil {
		panic(err)
	}
	result := make(RealmsConfigMap)
	for _, realmConfig := range realmsConfig {
		realmConfig.SecretEnv = utils.EnvOr(realmConfig.SecretEnvVarName)
		result[realmConfig.Id] = realmConfig
	}

	return result

}
