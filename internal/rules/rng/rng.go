package rng

import (
	"crypto/sha256"
	"encoding/binary"
	"math/rand/v2"
)

// Key 标识一条独立的随机数流：RNG(namespace, entity, purpose)。
//
// 例如 rng("combat", battle_id, "hit")。不同 Key 的流互不影响，
// 因此安装无关 Mod 不会使其他随机结果整体漂移。
type Key struct {
	Namespace string
	Entity    string
	Purpose   string
}

// Stream 是一条确定性随机数流。
type Stream struct {
	key Key
	src *rand.PCG
	r   *rand.Rand
}

// New 基于世界种子与 Key 派生一条确定性流。
// 相同 (seed, key) 永远得到相同序列。
func New(worldSeed uint64, key Key) *Stream {
	h := sha256.New()
	var seedBuf [8]byte
	binary.LittleEndian.PutUint64(seedBuf[:], worldSeed)
	h.Write(seedBuf[:])
	// 使用长度前缀避免 ("ab","c") 与 ("a","bc") 冲突。
	for _, part := range []string{key.Namespace, key.Entity, key.Purpose} {
		var l [8]byte
		binary.LittleEndian.PutUint64(l[:], uint64(len(part)))
		h.Write(l[:])
		h.Write([]byte(part))
	}
	sum := h.Sum(nil)
	src := rand.NewPCG(binary.LittleEndian.Uint64(sum[0:8]), binary.LittleEndian.Uint64(sum[8:16]))
	return &Stream{key: key, src: src, r: rand.New(src)} //nolint:gosec // 游戏确定性随机数，非安全用途
}

// Key 返回流的标识。
func (s *Stream) Key() Key { return s.key }

// Uint64 返回下一个 64 位随机数。
func (s *Stream) Uint64() uint64 { return s.r.Uint64() }

// IntN 返回 [0, n) 的随机整数。n 必须 > 0。
func (s *Stream) IntN(n int) int { return s.r.IntN(n) }

// Roll 掷一个 sides 面骰，返回 [1, sides]。
func (s *Stream) Roll(sides int) int { return s.r.IntN(sides) + 1 }

// MarshalBinary 序列化流的内部状态，用于存档与 Replay。
func (s *Stream) MarshalBinary() ([]byte, error) { return s.src.MarshalBinary() }

// UnmarshalBinary 恢复流的内部状态。
func (s *Stream) UnmarshalBinary(data []byte) error { return s.src.UnmarshalBinary(data) }
