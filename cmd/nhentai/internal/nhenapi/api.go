package nhenapi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"

	"github.com/SirZenith/delite/cmd/nhentai/internal/nhenapi/apipath"
)

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
	req, err := http.NewRequest("GET", url, body)
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

// GetBook fetches book info for given book id, any preceeding `#` will be removed
// from book id string.
func (c *NhenClient) GetBook(bookID int) (*Book, error) {
	target := apipath.Book(bookID)

	resp, err := c.Do("GET", target, nil)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	book, err := NewBookFromJSON(data)
	if err != nil {
		print(string(data))
		return nil, err
	}

	return book, nil
}
