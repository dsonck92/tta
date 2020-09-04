package tta

import (
	"io"
	"os"

	"github.com/dsonck92/tta/filter"
	"github.com/dsonck92/tta/wave"
)

type Decoder struct {
	codecs      [maxNCH]codec // 1 per channel
	channels    int           // number of channels/codecs
	data        [8]byte       // codec initialization data
	fifo        fifo
	passwordSet bool     // password protection flag
	seekAllowed bool     // seek table flag
	seekTable   []uint64 // the playing position table
	format      uint32   // tta data format
	rate        uint32   // bitrate (kbps)
	offset      uint64   // data start position (header size, bytes)
	frames      uint32   // total count of frames
	depth       uint32   // bytes per sample
	flenStd     uint32   // default frame length in samples
	flenLast    uint32   // last frame length in samples
	flen        uint32   // current frame length in samples
	fnum        uint32   // currently playing frame index
	fpos        uint32   // the current position in frame
}

func Decompress(infile io.ReadWriteSeeker, outfile io.WriteSeeker, passwd string, cb Callback) (err error) {
	decoder := MakeDecoder(infile)
	if len(passwd) > 0 {
		decoder.SetPassword(passwd)
	}
	info := Info{}
	if err = decoder.GetInfo(&info, 0); err != nil {
		return
	}
	smpSize := info.Nch * ((info.Bps + 7) / 8)
	dataSize := info.Samples * smpSize
	waveHdr := wave.NewHeader(dataSize, uint16(info.Nch), info.Sps, uint16(info.Bps), uint16(smpSize))
	if _, err = waveHdr.WriteTo(outfile); err != nil {
		return
	}
	bufSize := pcmBufferLength * smpSize
	buffer := make([]byte, bufSize)
	var writeLen int
	for {
		if writeLen = int(uint32(decoder.ProcessStream(buffer, cb)) * smpSize); writeLen == 0 {
			break
		}
		buf := buffer[:writeLen]
		if writeLen, err = outfile.Write(buf); err != nil {
			return
		} else if writeLen != len(buf) {
			err = errPartialWritten
			return
		}
	}
	return
}

func MakeDecoder(iocb io.ReadWriteSeeker) *Decoder {
	dec := Decoder{}
	dec.fifo.io = iocb
	return &dec
}

func (d *Decoder) ProcessStream(out []byte, cb Callback) int32 {
	var cache [maxNCH]int32
	var value int32
	var ret int32
	i := 0
	outClone := out[:]
	for d.fpos < d.flen && len(outClone) > 0 {
		value = d.fifo.getValue(&d.codecs[i].adapter)
		// decompress stage 1: adaptive hybrid filter
		d.codecs[i].filter.Decode(&value)
		// decompress stage 2: fixed order 1 prediction
		value += ((d.codecs[i].prev * ((1 << 5) - 1)) >> 5)
		d.codecs[i].prev = value
		cache[i] = value
		if i < d.channels-1 {
			i++
		} else {
			if d.channels == 1 {
				writeBuffer(value, outClone, d.depth)
				outClone = outClone[d.depth:]
			} else {
				k := i - 1
				cache[i] += cache[k] / 2
				for k > 0 {
					cache[k] = cache[k+1] - cache[k]
					k--
				}
				cache[k] = cache[k+1] - cache[k]
				for k <= i {
					writeBuffer(cache[k], outClone, d.depth)
					outClone = outClone[d.depth:]
					k++
				}
			}
			i = 0
			d.fpos++
			ret++
		}
		if d.fpos == d.flen {
			// check frame crc
			crcFlag := !d.fifo.readCrc32()
			if crcFlag {
				for i := 0; i < len(out); i++ {
					out[i] = 0
				}
				if !d.seekAllowed {
					break
				}
			}
			d.fnum++

			// update dynamic info
			d.rate = (d.fifo.count << 3) / 1070
			if cb != nil {
				cb(d.rate, d.fnum, d.frames)
			}
			if d.fnum == d.frames {
				break
			}
			d.frameInit(d.fnum, crcFlag)
		}
	}
	return ret
}

func (d *Decoder) ProcessFrame(inSize uint32, out []byte) int32 {
	i := 0
	var cache [maxNCH]int32
	var value int32
	var ret int32
	outClone := out[:]
	for d.fifo.count < inSize && len(outClone) > 0 {
		value = d.fifo.getValue(&d.codecs[i].adapter)
		// decompress stage 1: adaptive hybrid filter
		d.codecs[i].filter.Decode(&value)
		// decompress stage 2: fixed order 1 prediction
		value += ((d.codecs[i].prev * ((1 << 5) - 1)) >> 5)
		d.codecs[i].prev = value
		cache[i] = value
		if i < d.channels-1 {
			i++
		} else {
			if d.channels == 1 {
				writeBuffer(value, outClone, d.depth)
				outClone = outClone[d.depth:]
			} else {
				j := i
				k := i - 1
				cache[i] += cache[k] / 2
				for k > 0 {
					cache[k] = cache[j] - cache[k]
					j--
					k--
				}
				cache[k] = cache[j] - cache[k]
				for k <= i {
					writeBuffer(cache[k], outClone, d.depth)
					outClone = outClone[d.depth:]
					k++
				}
			}
			i = 0
			d.fpos++
			ret++
		}

		if d.fpos == d.flen || d.fifo.count == inSize-4 {
			// check frame crc
			if !d.fifo.readCrc32() {
				for i := 0; i < len(out); i++ {
					out[i] = 0
				}
			}
			// update dynamic info
			d.rate = (d.fifo.count << 3) / 1070
			break
		}
	}
	return ret
}

func (d *Decoder) readSeekTable() bool {
	if d.seekTable == nil {
		return false
	}
	d.fifo.reset()
	tmp := d.offset + uint64(d.frames+1)*4
	for i := uint32(0); i < d.frames; i++ {
		d.seekTable[i] = tmp
		tmp += uint64(d.fifo.readUint32())
	}
	return d.fifo.readCrc32()
}

func (d *Decoder) SetPassword(pass string) {
	d.data = computeKeyDigits(convertPassword(pass))
	d.passwordSet = true
}

func (d *Decoder) frameInit(frame uint32, seekNeeded bool) (err error) {
	if frame >= d.frames {
		return
	}
	shift := shifts[d.depth-1]
	d.fnum = frame
	if seekNeeded && d.seekAllowed {
		pos := d.seekTable[d.fnum]
		if pos != 0 {
			if _, err = d.fifo.io.Seek(int64(pos), os.SEEK_SET); err != nil {
				return errSeek
			}
		}
		d.fifo.readStart()
	}
	if d.fnum == d.frames-1 {
		d.flen = d.flenLast
	} else {
		d.flen = d.flenStd
	}
	for i := 0; i < d.channels; i++ {
		d.codecs[i].filter = filter.New(d.data, shift)
		d.codecs[i].adapter.init(10, 10)
		d.codecs[i].prev = 0
	}
	d.fpos = 0
	d.fifo.reset()
	return
}

func (d *Decoder) frameReset(frame uint32, iocb io.ReadWriteSeeker) {
	d.fifo.io = iocb
	d.fifo.readStart()
	d.frameInit(frame, false)
}

func (d *Decoder) SetPosition(seconds uint32) (newPos uint32, err error) {
	var frame = (245 * (seconds) / 256)
	newPos = (256 * (frame) / 245)
	if !d.seekAllowed || frame >= d.frames {
		err = errSeek
		return
	}
	d.frameInit(frame, true)
	return
}

func (d *Decoder) ReadHeader(info *Info) (uint32, error) {
	size := d.fifo.skipId3v2()
	d.fifo.reset()
	if 'T' != d.fifo.readByte() ||
		'T' != d.fifo.readByte() ||
		'A' != d.fifo.readByte() ||
		'1' != d.fifo.readByte() {
		return 0, errFormat
	}
	info.Format = uint32(d.fifo.readUint16())
	info.Nch = uint32(d.fifo.readUint16())
	info.Bps = uint32(d.fifo.readUint16())
	info.Sps = d.fifo.readUint32()
	info.Samples = d.fifo.readUint32()
	if !d.fifo.readCrc32() {
		return 0, errFile
	}
	size += 22
	return size, nil
}

func (d *Decoder) GetInfo(info *Info, pos int64) (err error) {
	if pos != 0 {
		if _, err = d.fifo.io.Seek(pos, os.SEEK_SET); err != nil {
			err = errSeek
			return
		}
	}
	d.fifo.readStart()
	var p uint32
	if p, err = d.ReadHeader(info); err != nil {
		return
	}
	if info.Format > 2 ||
		info.Bps < minBPS ||
		info.Bps > maxBPS ||
		info.Nch > maxNCH {
		return errFormat
	}
	if info.Format == formatEncrypted {
		if !d.passwordSet {
			return errPassword
		}
	} else {
		// disregard password if file is not encrypted
		d.passwordSet = false
		d.data = [8]byte{}
	}
	d.offset = uint64(pos) + uint64(p)
	d.format = info.Format
	d.depth = (info.Bps + 7) / 8
	d.flenStd = (256 * (info.Sps) / 245)
	d.flenLast = info.Samples % d.flenStd
	d.frames = info.Samples / d.flenStd
	if d.flenLast != 0 {
		d.frames++
	} else {
		d.flenLast = d.flenStd
	}
	d.rate = 0
	d.seekTable = make([]uint64, d.frames)
	d.seekAllowed = d.readSeekTable()
	d.channels = int(info.Nch)
	d.frameInit(0, false)
	return
}
