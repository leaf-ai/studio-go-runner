package vault

import (
	"context"
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

type VaultReferenceRoot struct {
	Ref *VaultReference `json:"vault"`
}

func (vr *VaultReference) Resolve() (key string, secret string, region string, err kv.Error) {
	config := vault.DefaultConfig()
	config.Address = vr.Endpoint

	defer func() {
		if err != nil {
			err = err.With("server", vr.Endpoint).With("path", vr.Secret)
		}
	}()

	client, vErr := vault.NewClient(config)
	if vErr != nil {
		return "", "", "", kv.Wrap(vErr)
	}
	if vaultToken == nil || *vaultToken == "" {
		return "", "", "",
			kv.NewError("Access Vault token is not specified")
	}
	client.SetToken(*vaultToken)
	data, vErr := client.KVv2("secret").Get(context.Background(), vr.Secret)
	if vErr != nil {
		return "", "", "", kv.Wrap(vErr)
	}

	credData := data.Data
	if credData == nil {
		return "", "", "",
			kv.NewError("Secret data not found")
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

func (vr *VaultReferenceRoot) Clone() *VaultReferenceRoot {
	return &VaultReferenceRoot{
		Ref: vr.Ref.Clone(),
	}
}

func (vr *VaultReference) Clone() *VaultReference {
	return &VaultReference{
		Endpoint: vr.Endpoint[:],
		Auth: &VaultAuthMethod{
			Method: vr.Auth.Method[:],
			Token:  vr.Auth.Token[:],
		},
		Secret: vr.Secret[:],
	}
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
