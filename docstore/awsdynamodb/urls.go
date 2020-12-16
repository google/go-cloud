// Copyright 2019 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package awsdynamodb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/aws/aws-sdk-go/aws/client"
	dyn "github.com/aws/aws-sdk-go/service/dynamodb"
	gcaws "gocloud.dev/aws"
	"gocloud.dev/docstore"
)

func init() {
	docstore.DefaultURLMux().RegisterCollection(Scheme, new(lazySessionOpener))
}

type lazySessionOpener struct {
	init   sync.Once
	opener *URLOpener
	err    error
}

func (o *lazySessionOpener) OpenCollectionURL(ctx context.Context, u *url.URL) (*docstore.Collection, error) {
	o.init.Do(func() {
		sess, err := gcaws.NewDefaultSession()
		if err != nil {
			o.err = err
			return
		}
		o.opener = &URLOpener{
			ConfigProvider: sess,
		}
	})
	if o.err != nil {
		return nil, fmt.Errorf("open collection %s: %v", u, o.err)
	}
	return o.opener.OpenCollectionURL(ctx, u)
}

// Scheme is the URL scheme dynamodb registers its URLOpener under on
// docstore.DefaultMux.
const Scheme = "dynamodb"

// URLOpener opens dynamodb URLs like
// "dynamodb://mytable?partition_key=partkey&sort_key=sortkey".
//
// The URL Host is used as the table name. See
// https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/HowItWorks.NamingRulesDataTypes.html
// for more details.
//
// The following query parameters are supported:
//
//   - partition_key (required): the path to the partition key of a table or an index.
//   - sort_key: the path to the sort key of a table or an index.
//   - allow_scans: if "true", allow table scans to be used for queries
//   - consistent_read: if "true", a strongly consistent read is used whenever possible.
//
// See https://godoc.org/gocloud.dev/aws#ConfigFromURLParams for supported query
// parameters for overriding the aws.Session from the URL.
type URLOpener struct {
	// ConfigProvider must be set to a non-nil value.
	ConfigProvider client.ConfigProvider
}

// OpenCollectionURL opens the collection at the URL's path. See the package doc for more details.
func (o *URLOpener) OpenCollectionURL(_ context.Context, u *url.URL) (*docstore.Collection, error) {
	db, tableName, partitionKey, sortKey, opts, err := o.processURL(u)
	if err != nil {
		return nil, err
	}
	return OpenCollection(db, tableName, partitionKey, sortKey, opts)
}

func (o *URLOpener) processURL(u *url.URL) (db *dyn.DynamoDB, tableName, partitionKey, sortKey string, opts *Options, err error) {
	q := u.Query()

	partitionKey = q.Get("partition_key")
	if partitionKey == "" {
		return nil, "", "", "", nil, fmt.Errorf("open collection %s: partition_key is required to open a table", u)
	}
	q.Del("partition_key")
	sortKey = q.Get("sort_key")
	q.Del("sort_key")
	opts = &Options{
		AllowScans:     q.Get("allow_scans") == "true",
		RevisionField:  q.Get("revision_field"),
		ConsistentRead: q.Get("consistent_read") == "true",
	}
	q.Del("allow_scans")
	q.Del("revision_field")
	q.Del("consistent_read")

	tableName = u.Host
	if tableName == "" {
		return nil, "", "", "", nil, fmt.Errorf("open collection %s: URL's host cannot be empty (the table name)", u)
	}
	if u.Path != "" {
		return nil, "", "", "", nil, fmt.Errorf("open collection %s: URL path must be empty, only the host is needed", u)
	}

	configProvider := &gcaws.ConfigOverrider{
		Base: o.ConfigProvider,
	}
	overrideCfg, err := gcaws.ConfigFromURLParams(q)
	if err != nil {
		return nil, "", "", "", nil, fmt.Errorf("open collection %s: %v", u, err)
	}
	configProvider.Configs = append(configProvider.Configs, overrideCfg)
	db, err = Dial(configProvider)
	if err != nil {
		return nil, "", "", "", nil, fmt.Errorf("open collection %s: %v", u, err)
	}
	return db, tableName, partitionKey, sortKey, opts, nil
}

// Dial gets an AWS DynamoDB service client.
func Dial(p client.ConfigProvider) (*dyn.DynamoDB, error) {
	if p == nil {
		return nil, errors.New("getting Dynamo service: no AWS session provided")
	}
	return dyn.New(p), nil
}
