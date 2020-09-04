# tta
[![Go Report Card](https://goreportcard.com/badge/github.com/dsonck92/tta)](https://goreportcard.com/report/github.com/dsonck92/tta)
[![GoDoc](https://godoc.org/github.com/dsonck92/tta?status.svg)](https://godoc.org/github.com/dsonck92/tta)
[![Build Status](https://travis-ci.org/dsonck92/tta.svg?branch=master)](https://travis-ci.org/dsonck92/tta)
[![Coverage Status](https://coveralls.io/repos/github/dsonck92/tta/badge.svg?branch=master)](https://coveralls.io/github/dsonck92/tta?branch=master)

[TTA Lossless Audio Codec](http://en.true-audio.com/TTA_Lossless_Audio_Codec_-_Realtime_Audio_Compressor) Encoder/Decoder for #golang

## `gotta` console tool

- install: `go get github.com/zyxar/tta/cmd/gotta`
- usage:

  ```
    -decode=false: decode file
    -encode=false: encode file
    -help=false: print this help
    -passwd="": specify password
  ```

## TODOs

- [x] SSE4 acceleration
- [ ] general optimization
