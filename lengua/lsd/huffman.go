package lsd

import (
	"errors"
	"fmt"
)

var errBadCode = errors.New("lsd: invalid huffman code")

// huffmanTable is a prefix code described only by code lengths ("LenTable"
// in ABBYY terminology). The file stores, for each symbol index, the length
// of its code; the tree is rebuilt by placing every symbol at the leftmost
// free slot of the requested depth.
//
// Child links use the encoding of the reference implementation:
// 0 = empty, >0 = (inner node index)+1, <0 = -(symbol index)-1.
type huffmanTable struct {
	nodes []huffmanNode // the root is the last node
}

type huffmanNode struct {
	left, right int32
}

// readHuffmanTable reads a table definition from the bit stream.
func readHuffmanTable(r *bitReader) (*huffmanTable, error) {
	count := r.bits(32)
	bitsPerLen := uint(r.bits(8))
	if r.err() != nil {
		return nil, r.err()
	}
	if count < 2 || count > 1<<24 {
		return nil, fmt.Errorf("lsd: invalid huffman table size %d", count)
	}
	idxBits := bitLength(count)

	t := &huffmanTable{nodes: make([]huffmanNode, count-1)}
	b := treeBuilder{table: t, root: int32(count - 2)}
	for range count {
		sym := int32(r.bits(idxBits))
		length := int(r.bits(bitsPerLen))
		if r.err() != nil {
			return nil, r.err()
		}
		if length == 0 || sym >= int32(count) || !b.place(sym, b.root, length) {
			return nil, fmt.Errorf("lsd: corrupt huffman table (symbol %d, length %d)", sym, length)
		}
	}
	return t, nil
}

type treeBuilder struct {
	table *huffmanTable
	root  int32
	used  int32 // number of inner nodes allocated so far (excluding the root)
}

// place inserts symbol sym as a leaf at depth length below node idx.
func (b *treeBuilder) place(sym, idx int32, length int) bool {
	n := &b.table.nodes[idx]
	if length == 1 {
		switch {
		case n.left == 0:
			n.left = -1 - sym
		case n.right == 0:
			n.right = -1 - sym
		default:
			return false
		}
		return true
	}
	for _, child := range []*int32{&n.left, &n.right} {
		if *child == 0 {
			if b.used >= b.root {
				return false
			}
			b.used++
			*child = b.used
		}
		if *child > 0 && b.place(sym, *child-1, length-1) {
			return true
		}
	}
	return false
}

// decode reads one code from r and returns the symbol index.
func (t *huffmanTable) decode(r *bitReader) (int, error) {
	idx := int32(len(t.nodes) - 1)
	for range 64 {
		n := t.nodes[idx]
		child := n.left
		if r.bit() == 1 {
			child = n.right
		}
		if r.err() != nil {
			return 0, r.err()
		}
		switch {
		case child < 0:
			return int(-1 - child), nil
		case child == 0 || int(child) > len(t.nodes):
			return 0, errBadCode
		}
		idx = child - 1
	}
	return 0, errBadCode
}
