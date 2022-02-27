package generate

import (
	cmap "github.com/orcaman/concurrent-map"
)

type LoadRegistry struct {
	Registry cmap.ConcurrentMap
}
