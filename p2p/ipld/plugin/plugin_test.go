package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	shell "github.com/ipfs/go-ipfs-api"
	"github.com/lazyledger/nmt"
)

func TestDataSquareRowOrColumnRawInputParserCidEqNmtRoot(t *testing.T) {
	tests := []struct {
		name     string
		leafData [][]byte
	}{
		{"16 leaves", generateRandNamespacedRawData(16, namespaceSize, shareSize)},
		// TODO add at least a row of an extended data square (incl. parity bytes) as a test-vector too
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			n := nmt.New(sha256.New())
			buf := createByteBufFromRawData(t, tt.leafData)
			for _, share := range tt.leafData {
				err := n.Push(share[:namespaceSize], share[namespaceSize:])
				if err != nil {
					t.Errorf("nmt.Push() unexpected error = %v", err)
					return
				}
			}
			gotNodes, err := DataSquareRowOrColumnRawInputParser(buf, 0, 0)
			if err != nil {
				t.Errorf("DataSquareRowOrColumnRawInputParser() unexpected error = %v", err)
				return
			}
			rootNodeCid := gotNodes[0].Cid()
			multiHashOverhead := 2
			lastNodeHash := rootNodeCid.Hash()
			if got, want := lastNodeHash[multiHashOverhead:], n.Root().Bytes(); !bytes.Equal(got, want) {
				t.Errorf("hashes don't match\ngot: %v\nwant: %v", got, want)
			}
			lastNodeCid := gotNodes[len(gotNodes)-1].Cid()
			if gotHash, wantHash := lastNodeCid.Hash(), hashLeaf(tt.leafData[0]); !bytes.Equal(gotHash[multiHashOverhead:], wantHash) {
				t.Errorf("first node's hash does not match the Cid\ngot: %v\nwant: %v", gotHash[multiHashOverhead:], wantHash)
			}
			nodePrefixOffset := 1 // leaf / inner node prefix is one byte
			lastLeafNodeData := gotNodes[len(gotNodes)-1].RawData()
			if gotData, wantData := lastLeafNodeData[nodePrefixOffset:], tt.leafData[0]; !bytes.Equal(gotData, wantData) {
				t.Errorf("first node's data does not match the leaf's data\ngot: %v\nwant: %v", gotData, wantData)
			}
		})
	}
}

func TestDagPutWithPlugin(t *testing.T) {
	//t.Skip("Requires running ipfs daemon with the plugin compiled and installed")

	t.Log("Warning: running this test writes to your local IPFS block store!")

	const numLeaves = 32
	data := generateRandNamespacedRawData(numLeaves, namespaceSize, shareSize)
	buf := createByteBufFromRawData(t, data)
	printFirst := 10
	t.Logf("first leaf, nid: %x, data: %x...", data[0][:namespaceSize], data[0][namespaceSize:namespaceSize+printFirst])
	n := nmt.New(sha256.New())
	for _, share := range data {
		err := n.Push(share[:namespaceSize], share[namespaceSize:])
		if err != nil {
			t.Errorf("nmt.Push() unexpected error = %v", err)
			return
		}
	}

	sh := shell.NewLocalShell()
	cid, err := sh.DagPut(buf, "raw", DagParserFormatName)
	if err != nil {
		t.Fatal(err)
	}
	// convert NMT tree root to CID and verify it matches the CID returned by DagPut
	treeRootBytes := n.Root().Bytes()
	nmtCid := cidFromNamespacedSha256(treeRootBytes)
	if nmtCid.String() != cid {
		t.Errorf("CIDs do not match: got %v, want: %v", cid, nmtCid.String())
	}
	// print out cid s.t. it can be used on the commandline
	t.Logf("Stored with cid: %v\n", cid)

	// DagGet leaf by leaf:
	for i, wantShare := range data {
		gotLeaf := &nmtLeafNode{}
		path := leafIdxToPath(cid, i)
		if err := sh.DagGet(path, gotLeaf); err != nil {
			t.Errorf("DagGet(%s) failed: %v", path, err)
		}
		if gotShare := gotLeaf.Data; !bytes.Equal(gotShare, wantShare) {
			t.Errorf("DagGet returned different data than pushed, got: %v, want: %v", gotShare, wantShare)
		}
	}
}

func leafIdxToPath(cid string, idx int) string {
	// currently this fmt directive assumes 32 leaves:
	bin := fmt.Sprintf("%05b", idx)
	path := strings.Join(strings.Split(bin, ""), "/")
	return cid + "/" + path
}

func createByteBufFromRawData(t *testing.T, leafData [][]byte) *bytes.Buffer {
	buf := bytes.NewBuffer(make([]byte, 0))
	for _, share := range leafData {
		_, err := buf.Write(share)
		if err != nil {
			t.Fatalf("buf.Write() unexpected error = %v", err)
			return nil
		}
	}
	return buf
}

// this snippet of the nmt internals is copied here:
func hashLeaf(data []byte) []byte {
	h := sha256.New()
	nID := data[:namespaceSize]
	toCommittToDataWithoutNID := data[namespaceSize:]

	res := append(append(make([]byte, 0), nID...), nID...)
	data = append([]byte{nmt.LeafPrefix}, toCommittToDataWithoutNID...)
	h.Write(data)
	return h.Sum(res)
}

func generateRandNamespacedRawData(total int, nidSize int, leafSize int) [][]byte {
	data := make([][]byte, total)
	for i := 0; i < total; i++ {
		nid := make([]byte, nidSize)
		rand.Read(nid)
		data[i] = nid
	}
	sortByteArrays(data)
	for i := 0; i < total; i++ {
		d := make([]byte, leafSize)
		rand.Read(d)
		data[i] = append(data[i], d...)
	}

	return data
}

func sortByteArrays(src [][]byte) {
	sort.Slice(src, func(i, j int) bool { return bytes.Compare(src[i], src[j]) < 0 })
}
