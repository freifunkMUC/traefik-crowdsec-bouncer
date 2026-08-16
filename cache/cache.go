// Package cache holds an in-memory copy of CrowdSec's active ban decisions,
// kept up to date by the CrowdSec decision stream (see controller.StartStream).
// It lets forwardAuth checks be answered locally instead of with a live LAPI
// call on every single request.
package cache

import (
	"net"
	"sync"

	"github.com/fbonalair/traefik-crowdsec-bouncer/model"
)

type rangeEntry struct {
	network *net.IPNet
	value   string
}

// DecisionCache is safe for concurrent use.
type DecisionCache struct {
	mu     sync.RWMutex
	ips    map[string]struct{}
	ranges []rangeEntry
}

func New() *DecisionCache {
	return &DecisionCache{
		ips: make(map[string]struct{}),
	}
}

// Apply adds newDecisions and removes deletedDecisions. Decisions with a
// scope other than "Ip"/"Range", or a value that doesn't parse, are skipped.
func (c *DecisionCache) Apply(newDecisions, deletedDecisions []model.Decision) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, d := range deletedDecisions {
		c.removeLocked(d)
	}
	for _, d := range newDecisions {
		c.addLocked(d)
	}
}

func (c *DecisionCache) addLocked(d model.Decision) {
	switch d.Scope {
	case "Ip":
		if ip := net.ParseIP(d.Value); ip != nil {
			c.ips[d.Value] = struct{}{}
		}
	case "Range":
		if _, network, err := net.ParseCIDR(d.Value); err == nil {
			c.ranges = append(c.ranges, rangeEntry{network: network, value: d.Value})
		}
	}
}

func (c *DecisionCache) removeLocked(d model.Decision) {
	switch d.Scope {
	case "Ip":
		delete(c.ips, d.Value)
	case "Range":
		filtered := c.ranges[:0]
		for _, r := range c.ranges {
			if r.value != d.Value {
				filtered = append(filtered, r)
			}
		}
		c.ranges = filtered
	}
}

// IsBanned reports whether clientIP matches an exact-IP decision or falls
// inside a banned range. An unparsable clientIP is never considered banned
// by this function; callers should already have validated it.
func (c *DecisionCache) IsBanned(clientIP string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if _, ok := c.ips[clientIP]; ok {
		return true
	}

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	for _, r := range c.ranges {
		if r.network.Contains(ip) {
			return true
		}
	}
	return false
}

// Size returns the number of cached exact-IP and range decisions, for
// metrics/logging.
func (c *DecisionCache) Size() (ips int, ranges int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.ips), len(c.ranges)
}
