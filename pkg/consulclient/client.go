package consulclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

// Client
type Client struct {
	baseURL    string
	HTTPClient *http.Client
	apiKey     string
}

// NewClient creates new Nomad client with given API key
func NewClient(consulEndpoint string, apiKey string) *Client {

	return &Client{
		apiKey:     apiKey,
		HTTPClient: http.DefaultClient,
		//HTTPClient: config.Client(oauth1.NoContext, token),
		baseURL: fmt.Sprintf("%s", consulEndpoint),
	}
}

// send sends the request
// Content-type and body should be already added to req
func (c *Client) send(ctx context.Context, method string, apiPath string, body interface{}, v interface{}) error {

	var err error
	var req *http.Request

	if method == http.MethodGet {
		req, err = http.NewRequestWithContext(
			ctx,
			method,
			fmt.Sprintf("%s%s", c.baseURL, apiPath),
			nil,
		)
		if err != nil {
			return err
		}

		//req.URL.RawQuery = params.Encode()
	} else {
		j, err := json.Marshal(body)
		if err != nil {
			return err
		}

		req, err = http.NewRequestWithContext(
			ctx,
			method,
			fmt.Sprintf("%s%s", c.baseURL, apiPath),
			strings.NewReader(string(j)),
		)
		if err != nil {
			return err
		}
	}

	return c.sendRequest(req, v)
}

func (c *Client) sendRequest(req *http.Request, v interface{}) error {
	//func (c *Client) sendRequest(req *http.Request, urlValues *url.Values, v interface{}) error {
	req.Header.Set("Accept", "application/json; charset=utf-8")
	req.Header.Set("Content-Type", "application/json")

	//authHeader := authHeader(req, params, c.apiKey)
	//req.Header.Set("Authorization", authHeader)

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	//debugBody(res)

	// Try to unmarshall into errorResponse
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		bodyBytes, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}

		return fmt.Errorf("unknown error, status code: %d, body: %s", res.StatusCode, string(bodyBytes))
	} else if res.StatusCode == http.StatusNoContent {
		return nil
	}

	if v != nil {
		if err = json.NewDecoder(res.Body).Decode(v); err != nil {
			return err
		}
	} else if err := res.Body.Close(); err != nil {
		return err
	}

	return nil
}

//func authHeader(req *http.Request, queryParams url.Values, apiKey string) string {
//	key := strings.SplitN(apiKey, ":", 3)
//	//config := oauth1.NewConfig(key[0], "")
//	//token := oauth1
//
//	auth := oauth1.OAuth1{
//		ConsumerKey:    key[0],
//		ConsumerSecret: "",
//		AccessToken:    key[1],
//		AccessSecret:   key[2],
//	}
//
//	//queryParams, _ := url.ParseQuery(req.URL.RawQuery)
//	params := make(map[string]string)
//	if req.Method != http.MethodPut {
//		// for some bizarre-reason PUT doesn't need this
//		for k, v := range queryParams {
//			params[k] = v[0]
//		}
//	}
//
//	authHeader := auth.BuildOAuth1Header(req.Method, req.URL.String(), params)
//	return authHeader
//}

func debugBody(res *http.Response) {
	bodyBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	bodyString := string(bodyBytes)

	fmt.Println(bodyString)
}
