package vault

import (
	"flag"
	vault "github.com/hashicorp/vault/api"
	"github.com/jjeffery/kv"
)

var (
	vaultToken = flag.String("vault-token", "", "Token for accessing Vault server")
)

type VaultAuthMethod struct {
	Method string `json:"method"`
	Token  string `json:"token"`
}

type VaultReference struct {
	Endpoint string           `json:"server"`
	Auth     *VaultAuthMethod `json:"auth"`
	Secret   string           `json:"path"`
}

func (vr *VaultReference) Resolve() (key string, secret string, region string, err kv.Error) {
	config := vault.DefaultConfig()
	config.Address = vr.Endpoint

	defer func() {
		err = err.With("server", vr.Endpoint).With("path", vr.Secret)
	}()

	client, vErr := vault.NewClient(config)
	if vErr != nil {
		return "", "", "", kv.Wrap(vErr)
	}
	if vaultToken == nil {
		return "", "", "",
			kv.NewError("Access Vault token is not specified")
	}
	client.SetToken(*vaultToken)
	data, vErr := client.Logical().Read(vr.Secret)
	if vErr != nil {
		return "", "", "", kv.Wrap(vErr)
	}

	credData, ok := data.Data["data"].(map[string]interface{})
	if !ok || credData == nil {
		return "", "", "",
			kv.NewError("Bad format of secret data")
	}

	key, err = getStrValue(credData, "access_key")
	if err != nil {
		return "", "", "", err
	}
	secret, err = getStrValue(credData, "secret_access_key")
	if err != nil {
		return "", "", "", err
	}
	region, err = getStrValue(credData, "region")
	if err != nil {
		return "", "", "", err
	}
	return key, secret, region, nil
}

func getStrValue(data map[string]interface{}, key string) (result string, err kv.Error) {
	x, ok := data[key]
	if !ok || x == nil {
		return "", kv.NewError("Data field is not found").With("key", key)
	}
	result, ok = x.(string)
	if !ok {
		return "", kv.NewError("String value expected").With("key", key)
	}
	return result, nil
}
