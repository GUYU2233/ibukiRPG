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

// String 返回 "namespace/entity/purpose" 形式，用作计数器键。
func (k Key) String() string { return k.Namespace + "/" + k.Entity + "/" + k.Purpose }

// New 基于世界种子与 Key 派生一条确定性流。
// 相同 (seed, key) 永远得到相同序列。
func New(worldSeed uint64, key Key) *Stream {
	return derive(worldSeed, key, nil)
}

// NewAt 派生同一 Key 的第 counter 次抽取所用的流。
//
// Game State 为每条命名流保存一个计数器（抽取次数），每次检定用 NewAt(seed, key, n)
// 取数后把计数器加一：这样重复检定得到不同结果，而存档 / Replay 只需记录计数器，
// 不需要序列化 PCG 内部状态；某条流被多用一次也不会影响其他流。
func NewAt(worldSeed uint64, key Key, counter uint64) *Stream {
	return derive(worldSeed, key, &counter)
}

func derive(worldSeed uint64, key Key, counter *uint64) *Stream {
	h := sha256.New()
	var seedBuf [8]byte
	binary.LittleEndian.PutUint64(seedBuf[:], worldSeed)
	h.Write(seedBuf[:])
	if counter != nil {
		// 带计数器的流使用不同的域前缀，与 New 的序列不重叠。
		var c [8]byte
		binary.LittleEndian.PutUint64(c[:], *counter)
		h.Write([]byte("ctr"))
		h.Write(c[:])
	}
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

// Counters 记录每条命名流已抽取的次数（存于 Game State，可序列化）。
type Counters map[string]uint64

// Next 取 key 的下一条流并递增计数器，返回流与本次使用的计数器值。
func (c Counters) Next(worldSeed uint64, key Key) (*Stream, uint64) {
	n := c[key.String()]
	c[key.String()] = n + 1
	return NewAt(worldSeed, key, n), n
}
