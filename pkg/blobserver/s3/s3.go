/*
Copyright 2011 The Perkeep Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
Package s3 registers the "s3" blobserver storage type, storing
blobs in an Amazon Web Services' S3 storage bucket.

Example low-level config:

     "/r1/": {
         "handler": "storage-s3",
         "handlerArgs": {
            "bucket": "foo",
            "aws_access_key": "...",
            "aws_secret_access_key": "...",
            "skipStartupCheck": false
          }
     },

*/
package s3 // import "perkeep.org/pkg/blobserver/s3"

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/hashicorp/hcl2/gohcl"

	"github.com/hashicorp/hcl2/hcl"

	"perkeep.org/pkg/configs"

	"perkeep.org/internal/amazon/s3"
	"perkeep.org/pkg/blob"
	"perkeep.org/pkg/blobserver"
	"perkeep.org/pkg/blobserver/memory"
	"perkeep.org/pkg/blobserver/proxycache"

	"go4.org/fault"
	"go4.org/jsonconfig"
	"go4.org/syncutil"
)

var (
	_ blob.SubFetcher               = (*s3Storage)(nil)
	_ blobserver.MaxEnumerateConfig = (*s3Storage)(nil)
)

var (
	faultReceive   = fault.NewInjector("s3_receive")
	faultEnumerate = fault.NewInjector("s3_enumerate")
	faultStat      = fault.NewInjector("s3_stat")
	faultGet       = fault.NewInjector("s3_get")
)

const maxParallelHTTP = 5

type s3Storage struct {
	s3Client *s3.Client
	bucket   string
	// optional "directory" where the blobs are stored, instead of at the root of the bucket.
	// S3 is actually flat, which in effect just means that all the objects should have this
	// dirPrefix as a prefix of their key.
	// If non empty, it should be a slash separated path with a trailing slash and no starting
	// slash.
	dirPrefix string
	hostname  string
}

func (s *s3Storage) String() string {
	if s.dirPrefix != "" {
		return fmt.Sprintf("\"s3\" blob storage at host %q, bucket %q, directory %q", s.hostname, s.bucket, s.dirPrefix)
	}
	return fmt.Sprintf("\"s3\" blob storage at host %q, bucket %q", s.hostname, s.bucket)
}

// NewFromHCLConfig would actually be newFromHCLConfig in a real implementation,
// and accessed only indirectly via the blobserver constructor registry.
func NewFromHCLConfig(l blobserver.Loader, config *configs.Storage) (blobserver.Storage, error) {
	type rawConfig struct {
		Bucket    hcl.Expression `hcl:"bucket"`
		Hostname  *string        `hcl:"hostname"`
		CacheSize *int64         `hcl:"cache_size"`

		AccessKeyID     hcl.Expression `hcl:"aws_access_key_id"`
		SecretAccessKey string         `hcl:"aws_secret_access_key"`

		SkipStartupCheck *bool `hcl:"skip_startup_check"`
	}

	var raw rawConfig
	diags := gohcl.DecodeBody(config.Config, config.EvalContext, &raw)
	if diags.HasErrors() {
		return nil, diags
	}

	hostname := "s3.amazonaws.com"
	if raw.Hostname != nil {
		hostname = *raw.Hostname
	}

	cacheSize := int64(32 << 20)
	if raw.CacheSize != nil {
		cacheSize = *raw.CacheSize
	}

	var accessKeyID string
	moreDiags := gohcl.DecodeExpression(raw.AccessKeyID, config.EvalContext, &accessKeyID)
	diags = append(diags, moreDiags...)
	accessKeyRange := raw.AccessKeyID.Range()
	if moreDiags.HasErrors() {
		return nil, diags
	}

	client := &s3.Client{
		Auth: &s3.Auth{
			AccessKey:       accessKeyID,
			SecretAccessKey: raw.SecretAccessKey,
			Hostname:        hostname,
		},
		PutGate: syncutil.NewGate(maxParallelHTTP),
		// TODO: optional transport, as in newFromConfigWithTransport below
	}

	var bucket string
	moreDiags = gohcl.DecodeExpression(raw.Bucket, config.EvalContext, &bucket)
	diags = append(diags, moreDiags...)
	bucketRange := raw.Bucket.Range()
	if moreDiags.HasErrors() {
		return nil, diags
	}

	var dirPrefix string
	if parts := strings.SplitN(bucket, "/", 2); len(parts) > 1 {
		dirPrefix = parts[1]
		bucket = parts[0]
	}
	if dirPrefix != "" && !strings.HasSuffix(dirPrefix, "/") {
		dirPrefix += "/"
	}

	sto := &s3Storage{
		s3Client:  client,
		bucket:    bucket,
		dirPrefix: dirPrefix,
		hostname:  hostname,
	}

	skipStartupCheck := false
	if raw.SkipStartupCheck != nil {
		skipStartupCheck = *raw.SkipStartupCheck
	}

	ctx := context.Background() // TODO: 5 min timeout or something?
	if !skipStartupCheck {
		_, err := client.ListBucket(ctx, sto.bucket, "", 1)
		if serr, ok := err.(*s3.Error); ok {
			switch serr.AmazonCode {
			case "NoSuchBucket":
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid S3 bucket",
					Detail:   fmt.Sprintf("There is no bucket named %q.", bucket),
					Subject:  &bucketRange, // report at the location of the bucket argument
				})
				return nil, diags
			case "InvalidAccessKeyId":
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid AWS Access Key ID",
					Detail:   fmt.Sprintf("The given access key id %q was refused by the S3 API.", accessKeyID),
					Subject:  &accessKeyRange, // report at the location of the access key argument
				})
				return nil, diags
			}

			// This code appears when the hostname has dots in it:
			if serr.AmazonCode == "PermanentRedirect" {
				loc, lerr := client.BucketLocation(ctx, sto.bucket)
				if lerr != nil {
					return nil, fmt.Errorf("Wrong server for bucket %q; and error determining bucket's location: %v", sto.bucket, lerr)
				}
				client.Auth.Hostname = loc
				_, err = client.ListBucket(ctx, sto.bucket, "", 1)
				if err == nil {
					log.Printf("Warning: s3 server should be %q, not %q. Change config file to avoid start-up latency.", client.Auth.Hostname, hostname)
				}
			}

			// This path occurs when the user set the
			// wrong server, or didn't set one at all, but
			// the bucket doesn't have dots in it:
			if serr.UseEndpoint != "" {
				// UseEndpoint will be e.g. "brads3test-ca.s3-us-west-1.amazonaws.com"
				// But we only want the "s3-us-west-1.amazonaws.com" part.
				client.Auth.Hostname = strings.TrimPrefix(serr.UseEndpoint, sto.bucket+".")
				_, err = client.ListBucket(ctx, sto.bucket, "", 1)
				if err == nil {
					log.Printf("Warning: s3 server should be %q, not %q. Change config file to avoid start-up latency.", client.Auth.Hostname, hostname)
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("Error listing bucket %s: %v", sto.bucket, err)
		}
	}

	if cacheSize != 0 {
		// This has two layers of LRU caching (proxycache and memory).
		// We make the outer one 4x the size so that it doesn't evict from the
		// underlying one when it's about to perform its own eviction.
		return proxycache.New(cacheSize<<2, memory.NewCache(cacheSize), sto), nil
	}
	return sto, nil
}

func newFromConfig(l blobserver.Loader, config jsonconfig.Obj) (blobserver.Storage, error) {
	return newFromConfigWithTransport(l, config, nil)
}

// newFromConfigWithTransport constructs a s3 blobserver using the given
// transport for all s3 requests.  The transport may be set to 'nil' to use a
// default transport.
// This is used for unit tests.
func newFromConfigWithTransport(_ blobserver.Loader, config jsonconfig.Obj, transport http.RoundTripper) (blobserver.Storage, error) {
	hostname := config.OptionalString("hostname", "s3.amazonaws.com")
	cacheSize := config.OptionalInt64("cacheSize", 32<<20)
	client := &s3.Client{
		Auth: &s3.Auth{
			AccessKey:       config.RequiredString("aws_access_key"),
			SecretAccessKey: config.RequiredString("aws_secret_access_key"),
			Hostname:        hostname,
		},
		PutGate:   syncutil.NewGate(maxParallelHTTP),
		Transport: transport,
	}
	bucket := config.RequiredString("bucket")
	var dirPrefix string
	if parts := strings.SplitN(bucket, "/", 2); len(parts) > 1 {
		dirPrefix = parts[1]
		bucket = parts[0]
	}
	if dirPrefix != "" && !strings.HasSuffix(dirPrefix, "/") {
		dirPrefix += "/"
	}
	sto := &s3Storage{
		s3Client:  client,
		bucket:    bucket,
		dirPrefix: dirPrefix,
		hostname:  hostname,
	}
	skipStartupCheck := config.OptionalBool("skipStartupCheck", false)
	if err := config.Validate(); err != nil {
		return nil, err
	}

	ctx := context.Background() // TODO: 5 min timeout or something?
	if !skipStartupCheck {
		_, err := client.ListBucket(ctx, sto.bucket, "", 1)
		if serr, ok := err.(*s3.Error); ok {
			if serr.AmazonCode == "NoSuchBucket" {
				return nil, fmt.Errorf("bucket %q doesn't exist", sto.bucket)
			}

			// This code appears when the hostname has dots in it:
			if serr.AmazonCode == "PermanentRedirect" {
				loc, lerr := client.BucketLocation(ctx, sto.bucket)
				if lerr != nil {
					return nil, fmt.Errorf("Wrong server for bucket %q; and error determining bucket's location: %v", sto.bucket, lerr)
				}
				client.Auth.Hostname = loc
				_, err = client.ListBucket(ctx, sto.bucket, "", 1)
				if err == nil {
					log.Printf("Warning: s3 server should be %q, not %q. Change config file to avoid start-up latency.", client.Auth.Hostname, hostname)
				}
			}

			// This path occurs when the user set the
			// wrong server, or didn't set one at all, but
			// the bucket doesn't have dots in it:
			if serr.UseEndpoint != "" {
				// UseEndpoint will be e.g. "brads3test-ca.s3-us-west-1.amazonaws.com"
				// But we only want the "s3-us-west-1.amazonaws.com" part.
				client.Auth.Hostname = strings.TrimPrefix(serr.UseEndpoint, sto.bucket+".")
				_, err = client.ListBucket(ctx, sto.bucket, "", 1)
				if err == nil {
					log.Printf("Warning: s3 server should be %q, not %q. Change config file to avoid start-up latency.", client.Auth.Hostname, hostname)
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("Error listing bucket %s: %v", sto.bucket, err)
		}
	}

	if cacheSize != 0 {
		// This has two layers of LRU caching (proxycache and memory).
		// We make the outer one 4x the size so that it doesn't evict from the
		// underlying one when it's about to perform its own eviction.
		return proxycache.New(cacheSize<<2, memory.NewCache(cacheSize), sto), nil
	}
	return sto, nil
}

func init() {
	blobserver.RegisterStorageConstructor("s3", blobserver.StorageConstructor(newFromConfig))
}
