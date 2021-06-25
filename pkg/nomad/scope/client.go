package scope

import (
	"github.com/hashicorp/nomad/api"
	"github.com/spectrocloud/cluster-api-provider-nomad/pkg/consulclient"
	"os"
)

// NewConsulClient creates a new Nomad client for a given session
func NewConsulClient(_ *ClusterScope) *consulclient.Client {

	consulEndpoint := os.Getenv("CONSUL_ENDPOINT")
	if consulEndpoint == "" {
		panic("missing env CONSUL_ENDPOINT; e.g: CONSUL_ENDPOINT=http://10.11.130.10:8500")
	}

	consulAPIKey := os.Getenv("CONSUL_API_KEY")

	client := consulclient.NewClient(consulEndpoint, consulAPIKey)

	return client
}

// NewNomadClient creates a new Nomad client for a given session
func NewNomadClient(_ *ClusterScope) *api.Client {

	nomadEndpoint := os.Getenv("NOMAD_ENDPOINT")
	if nomadEndpoint == "" {
		panic("missing env NOMAD_ENDPOINT; e.g: NOMAD_ENDPOINT=http://10.11.130.10:4646")
	}

	nomadAPIKey := os.Getenv("NOMAD_API_KEY")

	conf := api.DefaultConfig()
	conf.Address = nomadEndpoint
	conf.SecretID = nomadAPIKey
	conf.Region = "global"

	client, _ := api.NewClient(conf)

	return client

	// HTTP basic auth configuration.
	//httpAuth := d.Get("http_auth").(string)
	//if httpAuth != "" {
	//	var username, password string
	//	if strings.Contains(httpAuth, ":") {
	//		split := strings.SplitN(httpAuth, ":", 2)
	//		username = split[0]
	//		password = split[1]
	//	} else {
	//		username = httpAuth
	//	}
	//	conf.HttpAuth = &api.HttpBasicAuth{Username: username, Password: password}
	//}
	//
	//// TLS configuration items.
	//conf.TLSConfig.CACert = d.Get("ca_file").(string)
	//conf.TLSConfig.ClientCert = d.Get("cert_file").(string)
	//conf.TLSConfig.ClientKey = d.Get("key_file").(string)
	//conf.TLSConfig.CACertPEM = []byte(d.Get("ca_pem").(string))
	//conf.TLSConfig.ClientCertPEM = []byte(d.Get("cert_pem").(string))
	//conf.TLSConfig.ClientKeyPEM = []byte(d.Get("key_pem").(string))
	//
	// Set headers if provided
	//headers := d.Get("headers").([]interface{})
	//parsedHeaders := make(http.Header)
	//
	//for _, h := range headers {
	//	header := h.(map[string]interface{})
	//	if name, ok := header["name"]; ok {
	//		parsedHeaders.Add(name.(string), header["value"].(string))
	//	}
	//}
	//conf.Headers = parsedHeaders
	//
	//// Get the vault token from the conf, VAULT_TOKEN
	//// or ~/.vault-token (in that order)
	//var err error
	//vaultToken := d.Get("vault_token").(string)
	//if vaultToken == "" {
	//	vaultToken, err = getToken()
	//	if err != nil {
	//		return nil, err
	//	}
	//}
	//
	//consulToken := d.Get("consul_token").(string)

}
