package azcert

import (
	"context"
	"encoding/pem"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azcertificates"
	"strings"
)

const CertFunc = "azCert"

var azureClients = make(map[string]*azcertificates.Client, 10)

var cachedCerts = make(map[azcertificates.ID]*azcertificates.CertificateBundle, 5)

func ResolveCert(vaultName string, certArgs ...string) (any, error) {
	switch len(certArgs) {
	case 1:
		return getCertFromAzure(vaultName, certArgs[0], "")
	case 2:
		return getCertFromAzure(vaultName, certArgs[0], certArgs[1])
	default:
		return nil, fmt.Errorf("wrong arguments count for function %q expected 1 or 2 aruments but got %d", CertFunc, len(certArgs))
	}
}

func getCertFromAzure(vaultName string, certName string, certVer string) (string, error) {
	secretId := newId(vaultName, certName, certVer)
	cached := getFromCache(secretId)
	if cached != nil {
		return wrapCert(cached.CER), nil
	}
	client, err := getClient(vaultName)
	if err != nil {
		return "", err
	}
	certificate, err := client.GetCertificate(context.Background(), certName, certVer, nil)
	if err != nil {
		return "", err
	}
	saveToCache(*certificate.ID, &certificate.CertificateBundle)
	return wrapCert(certificate.CER), nil
}

func getClient(vaultName string) (*azcertificates.Client, error) {
	client := azureClients[vaultName]
	if client != nil {
		return client, nil
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	client, err = azcertificates.NewClient(getVaultUrl(vaultName), cred, nil)
	if err != nil {
		return nil, err
	}
	azureClients[vaultName] = client
	return client, nil
}

func newId(vaultName string, certName string, certVer string) azcertificates.ID {
	return azcertificates.ID(strings.Join([]string{getVaultUrl(vaultName), certName, certVer}, "/"))
}

func getVaultUrl(vaultName string) string {
	return fmt.Sprintf("https://%v.vault.azure.net", vaultName)
}

func getFromCache(id azcertificates.ID) *azcertificates.CertificateBundle {
	cached := cachedCerts[id]
	if cached == nil {
		return nil
	}
	return cached
}

func saveToCache(id azcertificates.ID, secret *azcertificates.CertificateBundle) {
	cachedCerts[id] = secret
}

func wrapCert(data []byte) string {
	block := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: data,
	}
	return string(pem.EncodeToMemory(block))
}
