package tta

import (
	"github.com/dsonck92/tta/filter"
)

type Info struct {
	Format  uint32 // audio format
	Nch     uint32 // number of channels
	Bps     uint32 // bits per sample
	Sps     uint32 // samplerate (sps)
	Samples uint32 // data length in samples
}

type adapter struct {
	k0   uint32
	k1   uint32
	sum0 uint32
	sum1 uint32
}

func (a *adapter) init(k0, k1 uint32) {
	a.k0 = k0
	a.k1 = k1
	a.sum0 = shift16[k0]
	a.sum1 = shift16[k1]
}

type codec struct {
	filter  *filter.Filter
	adapter adapter
	prev    int32
}

type Callback func(uint32, uint32, uint32)
