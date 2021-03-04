package coap

import (
	"errors"
	"math"
	"sync"
	"time"
)

type BlockMetadata struct {
	Size int
	More bool
	Num  int
}

func expireBlocks() {
	for {
		var toDel []string
		blockCache.Range(func(key, value interface{}) bool {
			bce, ok := value.(*blockCacheEntry)
			if ok && time.Now().After(bce.expires) {
				toDel = append(toDel, key.(string))
			}
			return true
		})
		for _, key := range toDel {
			blockCache.Delete(key)
		}
		time.Sleep(time.Second * 2)
	}

}

func blockDecode(i interface{}) (*BlockMetadata, error) {

	if i == nil {
		return nil, nil
	}

	buf, ok := i.([]byte)
	if !ok {
		return nil, errors.New("invalid interface type")
	}

	var bm BlockMetadata

	switch len(buf) {
	case 1:
		if (buf[0] & 0x08) == 0x08 {
			bm.More = true
		}
		bm.Size = int(math.Pow(2.0, 4.0+float64(buf[0]&0x07)))
		bm.Num = int(buf[0] >> 4)
	case 2:
		if (buf[1] & 0x08) == 0x08 {
			bm.More = true
		}
		bm.Size = int(math.Pow(2.0, 4.0+float64(buf[1]&0x07)))
		bm.Num = int(buf[0])<<4 + int(buf[1]>>4)
	case 3:
		if (buf[2] & 0x08) == 0x08 {
			bm.More = true
		}
		bm.Size = int(math.Pow(2.0, 4.0+float64(buf[2]&0x07)))
		bm.Num = int(buf[0])<<12 + int(buf[1])<<4 + int(buf[2]>>4)
	default:
		return nil, errors.New("blockwise metadata invalid length")
	}

	return &bm, nil
}

func blockInit(num int, more bool, sz int) *BlockMetadata {
	return &BlockMetadata{Size: sz, Num: num, More: more}
}

func (bm *BlockMetadata) Encode() []byte {
	sz := byte(0)
	switch bm.Size {
	case 16:
		sz = 0x00
	case 32:
		sz = 0x01
	case 64:
		sz = 0x02
	case 128:
		sz = 0x03
	case 256:
		sz = 0x04
	case 512:
		sz = 0x05
	case 1024:
		sz = 0x06
	case 2048:
		sz = 0x07
	}

	var buf []byte
	if bm.Num <= 7 {
		//1 byte
		buf = make([]byte, 1)
		buf[0] = byte((bm.Num << 4) & 0xFF)
	} else if bm.Num <= 4096 {
		//2 bytes
		buf = make([]byte, 2)
		buf[1] = byte((bm.Num << 4) & 0xFF)
		buf[0] = byte((bm.Num >> 4) & 0xFF)
	} else {
		//3 bytes
		buf = make([]byte, 3)
		buf[2] = byte((bm.Num << 4) & 0xFF)
		buf[1] = byte((bm.Num >> 4) & 0xFF)
		buf[0] = byte((bm.Num >> 12) & 0xFF)
	}
	if bm.More {
		buf[len(buf)-1] |= 0x08 | sz
	} else {
		buf[len(buf)-1] |= sz
	}
	return buf
}

type blockCacheEntry struct {
	rsp     *Message
	expires time.Time
}

var blockCache sync.Map

func blockCachePut(req *Message, key string) {
	if len(key) == 0 {
		key = req.getBlockKey()
	}
	blockCache.Store(key, &blockCacheEntry{rsp: req, expires: time.Now().Add(config.BlockInactivityTimeout)})
}

func blockCacheAppend(req *Message) error {
	bcei, ok := blockCache.Load(req.getBlockKey())
	if !ok {
		return errors.New("block not found")
	}
	bce := bcei.(*blockCacheEntry)
	bce.rsp.Payload = append(bce.rsp.Payload, req.Payload...)
	return nil
}

func blockCacheGet(req *Message, num int, sz int) (*Message, error) {
	bcei, ok := blockCache.Load(req.getBlockKey())
	if !ok {
		return nil, errors.New("block not found")
	}
	bce := bcei.(*blockCacheEntry)
	offset := num * sz
	if offset > len(bce.rsp.Payload) {
		return nil, errors.New("block overflow")
	}
	newRsp := *bce.rsp

	if num >= 0 {
		blockSize := 0
		more := false
		if offset+sz > len(bce.rsp.Payload) {
			blockSize = len(bce.rsp.Payload) - offset
			bce.expires = time.Now().Add(time.Second * 10)
		} else {
			blockSize = sz
			more = true
			bce.expires = time.Now().Add(config.BlockInactivityTimeout)
		}

		newRsp.Payload = bce.rsp.Payload[offset : offset+blockSize]
		if num == 0 {
			newRsp.WithSize2(len(bce.rsp.Payload))
		}

		bm := blockInit(num, more, sz)
		newRsp.WithBlock2(bm)
	} else {
		bce.expires = time.Now().Add(time.Second * 10)
		newRsp.Payload = bce.rsp.Payload[:]
		newRsp.Payload = append(newRsp.Payload, req.Payload...)
	}

	newRsp.Meta = Metadata{}
	newRsp.Token = req.Token
	newRsp.MessageID = req.MessageID

	return &newRsp, nil
}

func BlockCacheSize() (int64, int64) {
	size := int64(0)
	count := int64(0)
	blockCache.Range(func(key, value interface{}) bool {
		size += int64(len(value.(*Message).Payload))
		count++
		return true
	})
	return size, count
}
