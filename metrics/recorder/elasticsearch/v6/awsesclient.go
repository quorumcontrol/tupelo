package elasticsearch

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsexternal "github.com/aws/aws-sdk-go-v2/aws/external"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/elastic/go-elasticsearch/v6"
)

func NewAwsEsClient(url string) (*elasticsearch.Client, error) {
	cfg, err := awsexternal.LoadDefaultAWSConfig()
	if err != nil {
		return nil, err
	}

	if cfg.Region == "" {
		esURL, err := neturl.Parse(url)
		if err != nil {
			return nil, err
		}
		cfg.Region = strings.Split(esURL.Hostname(), ".")[1]
	}

	return elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{url},
		Transport: &AwsEsHttpSigner{
			RoundTripper: http.DefaultTransport,
			Credentials:  cfg.Credentials,
			Region:       cfg.Region,
		},
	})
}

// AwsEsHttpSigner is a http.RoundTripper implementation to sign requests according to
// https://docs.aws.amazon.com/general/latest/gr/signature-version-4.html. Many libraries allow customizing the behavior
// of HTTP requests, providing a transport. A AwsEsHttpSigner transport can be instantiated as follow:
//
// 	cfg, err := external.LoadDefaultAWSConfig()
//	if err != nil {
//		...
//	}
//	transport := &AwsEsHttpSigner{
//		RoundTripper: http.DefaultTransport,
//		Credentials:  cfg.Credentials,
//		Region:       cfg.Region,
//	}
type AwsEsHttpSigner struct {
	RoundTripper http.RoundTripper
	Credentials  aws.CredentialsProvider
	Region       string
}

func (s *AwsEsHttpSigner) RoundTrip(req *http.Request) (*http.Response, error) {
	signer := v4.NewSigner(s.Credentials)
	switch req.Body {
	case nil:
		_, err := signer.Sign(context.Background(), req, nil, "es", s.Region, time.Now())
		if err != nil {
			return nil, fmt.Errorf("error signing request: %w", err)
		}
	default:
		b, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_, err = signer.Sign(context.Background(), req, bytes.NewReader(b), "es", s.Region, time.Now())
		if err != nil {
			return nil, fmt.Errorf("error signing request: %w", err)
		}
		req.Body = ioutil.NopCloser(bytes.NewReader(b))
	}
	return s.RoundTripper.RoundTrip(req)
}
