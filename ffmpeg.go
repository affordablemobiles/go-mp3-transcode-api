package main

import (
	"errors"
	"fmt"
	"io"
	"log"

	gmf "github.com/3d0c/gmf"
)

func addStream(codecName string, oc *gmf.FmtCtx, ist *gmf.Stream) (int, int) {
	var cc *gmf.CodecCtx
	var ost *gmf.Stream

	codec := assert(gmf.FindEncoder(codecName)).(*gmf.Codec)

	// Create Video stream in output context
	if ost = oc.NewStream(codec); ost == nil {
		panic(errors.New("unable to create stream in output context"))
	}
	//defer gmf.Release(ost)

	if cc = gmf.NewCodecCtx(codec); cc == nil {
		panic(errors.New("unable to create codec context"))
	}
	//defer gmf.Release(cc)

	if oc.IsGlobalHeader() {
		cc.SetFlag(gmf.CODEC_FLAG_GLOBAL_HEADER)
	}

	if codec.IsExperimental() {
		cc.SetStrictCompliance(gmf.FF_COMPLIANCE_EXPERIMENTAL)
	}

	cc.SetSampleFmt(gmf.AV_SAMPLE_FMT_S16P)
	cc.SetSampleRate(ist.CodecCtx().SampleRate())
	cc.SetChannels(ist.CodecCtx().Channels())
	cc.SetChannelLayout(ist.CodecCtx().ChannelLayout())

	if err := cc.Open(nil); err != nil {
		panic(err)
	}

	ost.SetCodecCtx(cc)
	ost.DumpContexCodec(cc)

	return ist.Index(), ost.Index()
}

func transcode_audio(w io.Writer, r io.ReadCloser) {
	var _, outputIndex int

	// Input
	inputCtx := gmf.NewCtx()
	defer inputCtx.Free()

	iavioCtx, err := NewSourceReader(r).GetAVIOContext(inputCtx)
	if err != nil {
		panic(err)
	}
	defer gmf.Release(iavioCtx)
	inputCtx.SetPb(iavioCtx).OpenInput("")

	// Output
	ofmt := gmf.FindOutputFmt("", "output.mp3", "")
	if ofmt == nil {
		panic(fmt.Errorf("Failed to determine output format"))
	}
	defer ofmt.Free()
	ofmt.Filename = ""

	outputCtx, err := gmf.NewOutputCtx(ofmt, nil)
	if err != nil {
		panic(err)
	}
	defer outputCtx.Free()

	oavioCtx, err := NewDestinationWriter(w).GetAVIOContext(outputCtx)
	if err != nil {
		panic(err)
	}
	defer gmf.Release(oavioCtx)
	outputCtx.SetPb(oavioCtx)

	srcAudioStream, err := inputCtx.GetBestStream(gmf.AVMEDIA_TYPE_AUDIO)
	if err != nil {
		log.Printf("No audio stream found in input")
	} else {
		_, outputIndex = addStream("libmp3lame", outputCtx, srcAudioStream)
	}
	inputCodecCtx := srcAudioStream.CodecCtx()

	/// resample
	ost := assert(outputCtx.GetStream(outputIndex)).(*gmf.Stream)
	defer gmf.Release(ost.CodecCtx())
	defer gmf.Release(ost)

	options := []*gmf.Option{
		{Key: "in_channel_layout", Val: inputCodecCtx.ChannelLayout()},
		{Key: "out_channel_layout", Val: inputCodecCtx.ChannelLayout()},
		{Key: "in_sample_rate", Val: inputCodecCtx.SampleRate()},
		{Key: "out_sample_rate", Val: ost.CodecCtx().SampleRate()},
		{Key: "in_sample_fmt", Val: inputCodecCtx.SampleFmt()},
		{Key: "out_sample_fmt", Val: ost.CodecCtx().SampleFmt()},
	}

	swrCtx, err := gmf.NewSwrCtx(options, ost.CodecCtx().Channels(), ost.CodecCtx().SampleFmt())
	if err != nil {
		panic(fmt.Errorf("new swr context error: %s", err.Error()))
	}
	if swrCtx == nil {
		panic(fmt.Errorf("unable to create Swr Context"))
	}

	if err := outputCtx.WriteHeader(); err != nil {
		panic(err)
	}

	var (
		packets int = 0
		frames  int = 0
		encoded int = 0
	)

	for packet := range inputCtx.GetNewPackets() {
		func() {
			packets++

			//ist := assert(inputCtx.GetStream(0)).(*gmf.Stream)

			srcFrames, err := inputCodecCtx.Decode(packet)
			defer packet.Free()
			if err != nil {
				log.Printf("capture audio error: %s", err)
				return
			}

			for _, frame := range srcFrames {
				frames++

				func() {
					dstFrame, err := swrCtx.Convert(frame)
					if err != nil {
						log.Printf("convert audio error: %s", err)
						panic(err)
					}
					if dstFrame == nil {
						return
					}
					defer frame.Free()
					defer dstFrame.Free()

					pkt, err := dstFrame.Encode(ost.CodecCtx())
					if err != nil {
						log.Println(err)
						return
					}
					if pkt == nil {
						return
					}
					defer pkt.Free()

					pkt.SetStreamIndex(ost.Index())

					if err := outputCtx.WritePacket(pkt); err != nil {
						panic(err)
					}
				}()
			}

			encoded++
		}()
	}

	// Drain the encoder...
	for {
		pkts, err := ost.CodecCtx().Encode(nil, 1)
		if err != nil && err.Error() != "End of file" {
			panic(err)
		}
		if len(pkts) <= 0 {
			break
		}

		for _, pkt := range pkts {
			pkt.SetStreamIndex(ost.Index())

			if err := outputCtx.WritePacket(pkt); err != nil {
				panic(err)
			}

			pkt.Free()
		}
	}

	oavioCtx.Flush()

	log.Printf("packets: %d, frames: %d, encoded: %d\n", packets, frames, encoded)
}
