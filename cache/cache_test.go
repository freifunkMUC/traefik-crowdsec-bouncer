package cache

import (
	"testing"

	"github.com/fbonalair/traefik-crowdsec-bouncer/model"
	"github.com/stretchr/testify/assert"
)

func TestIsBannedExactIp(t *testing.T) {
	c := New()
	c.Apply([]model.Decision{{Scope: "Ip", Value: "1.2.3.4"}}, nil)

	assert.True(t, c.IsBanned("1.2.3.4"))
	assert.False(t, c.IsBanned("1.2.3.5"))
}

func TestIsBannedRange(t *testing.T) {
	c := New()
	c.Apply([]model.Decision{{Scope: "Range", Value: "5.6.7.0/24"}}, nil)

	assert.True(t, c.IsBanned("5.6.7.42"))
	assert.False(t, c.IsBanned("5.6.8.42"))
}

func TestApplyRemovesDeletedIp(t *testing.T) {
	c := New()
	c.Apply([]model.Decision{{Scope: "Ip", Value: "1.2.3.4"}}, nil)
	assert.True(t, c.IsBanned("1.2.3.4"))

	c.Apply(nil, []model.Decision{{Scope: "Ip", Value: "1.2.3.4"}})
	assert.False(t, c.IsBanned("1.2.3.4"))
}

func TestApplyRemovesDeletedRange(t *testing.T) {
	c := New()
	c.Apply([]model.Decision{{Scope: "Range", Value: "5.6.7.0/24"}}, nil)
	assert.True(t, c.IsBanned("5.6.7.42"))

	c.Apply(nil, []model.Decision{{Scope: "Range", Value: "5.6.7.0/24"}})
	assert.False(t, c.IsBanned("5.6.7.42"))
}

func TestApplyRemovesOneRangeKeepsOthers(t *testing.T) {
	c := New()
	c.Apply([]model.Decision{
		{Scope: "Range", Value: "5.6.7.0/24"},
		{Scope: "Range", Value: "8.8.8.0/24"},
	}, nil)

	c.Apply(nil, []model.Decision{{Scope: "Range", Value: "5.6.7.0/24"}})

	assert.False(t, c.IsBanned("5.6.7.42"))
	assert.True(t, c.IsBanned("8.8.8.42"))
}

func TestApplyIgnoresUnknownScope(t *testing.T) {
	c := New()
	c.Apply([]model.Decision{{Scope: "Country", Value: "FR"}}, nil)

	ips, ranges := c.Size()
	assert.Equal(t, 0, ips)
	assert.Equal(t, 0, ranges)
}

func TestApplyRemoveIgnoresUnknownScope(t *testing.T) {
	c := New()

	assert.NotPanics(t, func() {
		c.Apply(nil, []model.Decision{{Scope: "Country", Value: "FR"}})
	})
	ips, ranges := c.Size()
	assert.Equal(t, 0, ips)
	assert.Equal(t, 0, ranges)
}

func TestApplyIgnoresUnparsableValue(t *testing.T) {
	c := New()
	c.Apply([]model.Decision{
		{Scope: "Ip", Value: "not-an-ip"},
		{Scope: "Range", Value: "not-a-cidr"},
	}, nil)

	ips, ranges := c.Size()
	assert.Equal(t, 0, ips)
	assert.Equal(t, 0, ranges)
}

func TestIsBannedUnparsableClientIpIsFalse(t *testing.T) {
	c := New()
	c.Apply([]model.Decision{{Scope: "Range", Value: "5.6.7.0/24"}}, nil)

	assert.False(t, c.IsBanned("not-an-ip"))
}

func TestSize(t *testing.T) {
	c := New()
	c.Apply([]model.Decision{
		{Scope: "Ip", Value: "1.2.3.4"},
		{Scope: "Ip", Value: "1.2.3.5"},
		{Scope: "Range", Value: "5.6.7.0/24"},
	}, nil)

	ips, ranges := c.Size()
	assert.Equal(t, 2, ips)
	assert.Equal(t, 1, ranges)
}
