package nhenapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"reflect"
	"strings"
)

const MetaInfoFieldName = "Info"

func replaceWithPlaceholders(input string, dict map[string]string, leftDelim, rightDelim string) string {
	result := input
	for k, v := range dict {
		placeholder := leftDelim + k + rightDelim
		result = strings.ReplaceAll(result, placeholder, v)
	}
	return result
}

// fieldToString converts a reflect.Value to its string representation based on its kind.
func fieldToString(val reflect.Value) string {
	switch val.Kind() {
	case reflect.String:
		return val.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", val.Int())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%f", val.Float())
	case reflect.Bool:
		return fmt.Sprintf("%t", val.Bool())
	default:
		return fmt.Sprintf("%v", val.Interface())
	}
}

// parseRequest compose request infomation with given request argument value.
func parseRequest(arg any) (string, string, error) {
	v := reflect.ValueOf(arg)
	t := v.Type()

	infoField, ok := v.Type().FieldByName(MetaInfoFieldName)
	if !ok {
		return "", "", fmt.Errorf("field 'Info' not found in type %s", t.Name())
	}

	pathTemplate := infoField.Tag.Get("path")
	method := strings.ToUpper(infoField.Tag.Get("method"))

	argMap := map[string]string{}
	for i := range v.NumField() {
		field := t.Field(i)
		if field.Name == MetaInfoFieldName {
			continue
		}
		argMap[field.Name] = fieldToString(v.Field(i))
	}

	targetPath := replaceWithPlaceholders(pathTemplate, argMap, "${", "}")

	return targetPath, method, nil
}

type NhenClient struct {
	*http.Client
	headers map[string]string
}

func NewNhenClient() *NhenClient {
	client := &NhenClient{
		Client: new(http.Client),
	}

	client.Transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	return client
}

// SetProxy updates proxy URL used by client.
func (c *NhenClient) SetProxy(httpProxy, httpsProxy string) {
	transport, ok := c.Transport.(*http.Transport)
	if !ok {
		transport = new(http.Transport)
		c.Transport = transport
	}

	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		switch req.URL.Scheme {
		case "http":
			if httpProxy == "" {
				return nil, nil
			} else {
				return url.Parse(httpProxy)
			}
		case "https":
			if httpsProxy == "" {
				return nil, nil
			} else {
				return url.Parse(httpsProxy)
			}
		default:
			return http.ProxyFromEnvironment(req)
		}
	}
}

// SetCookie update cookie jar used by client.
func (c *NhenClient) SetCookie(urlStr string, cookies map[string]string) error {
	target, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("failed to parse URL %s: %s", urlStr, err)
	}

	if c.Jar == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return fmt.Errorf("failed to create cookie jar for new client: %s", err)
		}
		c.Jar = jar
	}

	list := []*http.Cookie{}
	for name, value := range cookies {
		list = append(list, &http.Cookie{
			Name: name, Value: value,
		})
	}

	c.Jar.SetCookies(target, list)

	return nil
}

// SetHeaders updates default headers used by all request.
func (c *NhenClient) SetHeaders(headers map[string]string) {
	result := map[string]string{}
	for k, v := range headers {
		result[k] = v
	}
	c.headers = result
}

// Do sends a new request with given method to `url`.
func (c *NhenClient) Do(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create new request: %s", err)
	}

	if c.headers != nil {
		for name, value := range c.headers {
			req.Header.Set(name, value)
		}
	}

	resp, err := c.Client.Do(req)
	return resp, err
}

func (c *NhenClient) ApiRequest(arg any, result any) error {
	v := reflect.ValueOf(result)
	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("`result` argument must be a pointer")
	}

	parsedPath, method, err := parseRequest(arg)
	if err != nil {
		return fmt.Errorf("failed to parse request argument: %s", err)
	}

	url := "http://" + path.Join(HostList[HostAPI], parsedPath)
	resp, err := c.Do(method, url, nil)
	if err != nil {
		return fmt.Errorf("failed to send request: %s", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %s", err)
	}

	// if `result` is not nil, try to unmarshal response data into it
	if !v.IsNil() {
		err = json.Unmarshal(data, result)
		if err != nil {
			return fmt.Errorf("failed to unmarshal response data: %s, data prints below\n%s\n%s", err, strings.Repeat("-", 20), string(data))
		}
	}

	return nil
}
