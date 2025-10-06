package common

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
)

func InvokeHttpRequest(r *model.HTTPArguments) (*resty.Response, error) {

	builder, err := CreateRequestBuilderFromEndpoint(r)

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"url": r.Endpoint.String(),
		}).WithError(err).Errorln("Failed to create request builder from endpoint")
		return nil, err
	}

	resp, err := MakeRequestFromBuilder(builder, r.Method, r.Endpoint.String())

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"url": r.Endpoint.String(),
		}).WithError(err).Errorln("Failed to fetch from URL")
		return nil, err
	}

	return resp, nil

}

func MakeRequestFromBuilder(restBuilder *resty.Request, method string, finalUrl string) (*resty.Response, error) {

	switch strings.ToUpper(method) {
	case http.MethodGet:
		return restBuilder.Get(finalUrl)
	case http.MethodPost:
		return restBuilder.Post(finalUrl)
	case http.MethodPut:
		return restBuilder.Put(finalUrl)
	case http.MethodDelete:
		return restBuilder.Delete(finalUrl)
	default:
		return nil, fmt.Errorf("unsupported HTTP method: %s. Ensure you're using the http const", method)
	}

}

func CreateRequestBuilderFromEndpoint(req *model.HTTPArguments) (*resty.Request, error) {
	if err := validateHTTPArguments(req); err != nil {
		return nil, err
	}

	client := resty.New()
	restBuilder := client.R().EnableTrace()

	if err := configureAuthentication(restBuilder, req.Endpoint); err != nil {
		return nil, err
	}

	configureRequestParameters(restBuilder, req)
	configureOutputFormat(restBuilder, req)
	configureRequestBody(restBuilder, req)

	return restBuilder, nil
}

func validateHTTPArguments(req *model.HTTPArguments) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if req.Endpoint == nil {
		return fmt.Errorf("endpoint is nil")
	}
	return nil
}

func configureAuthentication(restBuilder *resty.Request, uri *model.Endpoint) error {
	if uri.EndpointConfig == nil || uri.EndpointConfig.Authentication == nil {
		return nil
	}

	auth := uri.EndpointConfig.Authentication.AuthenticationPolicy

	switch {
	case auth.Basic != nil:
		restBuilder.SetBasicAuth(auth.Basic.Username, auth.Basic.Password)
	case auth.Bearer != nil:
		restBuilder.SetAuthToken(auth.Bearer.Token)
	case auth.Digest != nil:
		restBuilder.SetDigestAuth(auth.Digest.Username, auth.Digest.Password)
	default:
		logrus.WithFields(logrus.Fields{
			"url": uri.String(),
		}).Warnln("Unsupported authentication type in endpoint config")
	}

	return nil
}

func configureRequestParameters(restBuilder *resty.Request, req *model.HTTPArguments) {
	if req.Body != nil {
		restBuilder.SetBody(req.Body)
	}

	if len(req.Query) > 0 {
		for k, v := range req.Query {
			restBuilder.SetQueryParam(k, fmt.Sprintf("%v", v))
		}
	}

	if len(req.Headers) > 0 {
		restBuilder.SetHeaders(req.Headers)
	}
}

func configureOutputFormat(restBuilder *resty.Request, req *model.HTTPArguments) {
	/*
		The http call's output format.
			Supported values are:
			- raw, which output's the base-64 encoded http response content, if any.
			- content, which outputs the content of http response, possibly deserialized.
			- response, which outputs the http response.
			Defaults to content.
	*/
	if len(req.Output) == 0 {
		req.Output = "content" // Default to content
	}

	switch strings.ToLower(req.Output) {
	case "raw":
		restBuilder.SetDoNotParseResponse(true)
	case "content":
		// Default behavior - deserialize response
		fallthrough
	case "response":
		// object to hold response, headers, url etc
		var resultOut any
		restBuilder.SetResult(&resultOut)
	default:
		logrus.WithFields(logrus.Fields{
			"url":    req.Endpoint.String(),
			"output": req.Output,
		}).Warnln("Unsupported output type, defaulting to 'content'")
	}
}

func configureRequestBody(restBuilder *resty.Request, req *model.HTTPArguments) {
	if req.Body != nil {
		restBuilder.SetBody(req.Body).
			SetHeader("Content-Type", "application/json")
	}
}
