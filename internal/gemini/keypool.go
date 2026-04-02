package gemini

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type keyState struct {
	key        string
	usage      int
	cooldownAt time.Time
}

type KeyPool struct {
	mu               sync.Mutex
	keys             []*keyState
	cooldownDuration time.Duration
}

func NewKeyPool(keys []string, cooldownDuration time.Duration) *KeyPool {
	states := make([]*keyState, len(keys))
	for i, k := range keys {
		states[i] = &keyState{key: k}
	}
	return &KeyPool{
		keys:             states,
		cooldownDuration: cooldownDuration,
	}
}

func (p *KeyPool) GetKey() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.keys) == 0 {
		return "", fmt.Errorf("key pool is empty")
	}

	available := p.availableLocked()
	if len(available) > 0 {
		return p.weightedSelect(available), nil
	}

	soonest := p.soonestRecoveryLocked()
	wait := time.Until(soonest)
	if wait > 0 {
		p.mu.Unlock()
		time.Sleep(wait)
		p.mu.Lock()
	}

	available = p.availableLocked()
	if len(available) == 0 {
		return p.keys[rand.Intn(len(p.keys))].key, nil
	}
	return p.weightedSelect(available), nil
}

func (p *KeyPool) MarkCooldown(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, ks := range p.keys {
		if ks.key == key {
			ks.cooldownAt = time.Now().Add(p.cooldownDuration)
			return
		}
	}
}

func (p *KeyPool) Release(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, ks := range p.keys {
		if ks.key == key {
			ks.usage++
			return
		}
	}
}

func (p *KeyPool) Stats() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := make(map[string]int, len(p.keys))
	for _, ks := range p.keys {
		stats[ks.key] = ks.usage
	}
	return stats
}

func (p *KeyPool) availableLocked() []*keyState {
	now := time.Now()
	var available []*keyState
	for _, ks := range p.keys {
		if ks.cooldownAt.IsZero() || now.After(ks.cooldownAt) {
			available = append(available, ks)
		}
	}
	return available
}

func (p *KeyPool) soonestRecoveryLocked() time.Time {
	var soonest time.Time
	for _, ks := range p.keys {
		if !ks.cooldownAt.IsZero() {
			if soonest.IsZero() || ks.cooldownAt.Before(soonest) {
				soonest = ks.cooldownAt
			}
		}
	}
	return soonest
}

func (p *KeyPool) weightedSelect(available []*keyState) string {
	if len(available) == 1 {
		return available[0].key
	}

	weights := make([]float64, len(available))
	totalWeight := 0.0
	for i, ks := range available {
		w := 1.0 / float64(ks.usage+1)
		weights[i] = w
		totalWeight += w
	}

	r := rand.Float64() * totalWeight
	cumulative := 0.0
	for i, w := range weights {
		cumulative += w
		if r <= cumulative {
			return available[i].key
		}
	}
	return available[len(available)-1].key
}
