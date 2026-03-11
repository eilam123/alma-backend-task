package api

import "sync"

// responseCache holds cached API responses.
// Zero value is ready to use with empty (uncached) state.
// A generation counter prevents stale writes: if an invalidation occurs
// between a cache miss and the subsequent set call, the set is rejected.
//
// Callers must not modify the returned cached values — they are shared
// across all concurrent readers.
type responseCache struct {
	mu                sync.RWMutex
	generation        uint64
	catalog           *CatalogResponse
	connections       []ConnectionResponse
	connectionsCached bool // needed because nil slice is a valid empty result
}

// getCatalog returns the cached catalog and the current generation atomically.
// If the catalog is cached, the caller should use it directly.
// If not, the caller should use the returned generation when calling setCatalog.
func (c *responseCache) getCatalog() (*CatalogResponse, uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.catalog, c.generation
}

// setCatalog stores the catalog if the generation has not changed since the fetch started.
func (c *responseCache) setCatalog(gen uint64, v *CatalogResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation == gen && c.catalog == nil {
		c.catalog = v
	}
}

// getConnections returns the cached connections and the current generation atomically.
// If cached is true, the caller should use the value directly.
// If not, the caller should use the returned generation when calling setConnections.
func (c *responseCache) getConnections() ([]ConnectionResponse, uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connections, c.generation, c.connectionsCached
}

// setConnections stores the connections if the generation has not changed since the fetch started.
func (c *responseCache) setConnections(gen uint64, v []ConnectionResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation == gen && !c.connectionsCached {
		c.connections = v
		c.connectionsCached = true
	}
}

func (c *responseCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.catalog = nil
	c.connections = nil
	c.connectionsCached = false
}
