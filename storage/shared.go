package storage

import "github.com/dotvezz/mak-cache/cache"

var SharedStorageProviders = make(map[string]Provider[*cache.Entry])
var SharedMetadataProviders = make(map[string]Provider[*cache.Metadata])
