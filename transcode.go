package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	gmf "github.com/3d0c/gmf"
)

func websrvTranscodeForm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	file, err := os.Open("demo.html")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	io.Copy(w, file)
}

func websrvTranscode(w http.ResponseWriter, r *http.Request) {
	// Parse our multipart form, 10 << 20 specifies a maximum
	// upload of 10 MB files.
	r.ParseMultipartForm(100 << 20)

	// FormFile returns the first file for the given key `myFile`
	// it also returns the FileHeader so we can get the Filename,
	// the Header and the size of the file
	file, _, err := r.FormFile("source")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	websrvTranscodeResponse(w, r, file)
}

func websrvTranscodeResponse(w http.ResponseWriter, r *http.Request, file io.ReadCloser) {
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Accept-Ranges", "none")
	w.WriteHeader(http.StatusOK)

	// Input
	ictx := gmf.NewCtx()
	defer ictx.Free()

	iavioCtx, err := NewSourceReader(file).GetAVIOContext(ictx)
	defer gmf.Release(iavioCtx)
	if err != nil {
		panic(err)
	}
	ictx.SetPb(iavioCtx).OpenInput("")

	// Output
	ofmt := gmf.FindOutputFmt("", "output.mp3", "")
	if ofmt == nil {
		panic(fmt.Errorf("Failed to determine output format"))
	}
	defer ofmt.Free()
	ofmt.Filename = ""

	octx, err := gmf.NewOutputCtx(ofmt, nil)
	defer octx.Free()
	if err != nil {
		panic(err)
	}

	oavioCtx, err := NewDestinationWriter(w).GetAVIOContext(octx)
	defer gmf.Release(oavioCtx)
	if err != nil {
		panic(err)
	}
	octx.SetPb(oavioCtx)

	ast, err := ictx.GetBestStream(gmf.AVMEDIA_TYPE_AUDIO)
	if err != nil {
		panic(fmt.Errorf("failed to find audio stream"))
	}
	cc := ast.CodecCtx()

	/// fifo
	fifo := gmf.NewAVAudioFifo(cc.SampleFmt(), cc.Channels(), 1024)
	if fifo == nil {
		panic(fmt.Errorf("failed to create audio fifo"))
	}

	codec, err := gmf.FindEncoder("libmp3lame")
	if err != nil {
		panic(fmt.Errorf("find encoder error: %s", err.Error()))
	}

	audioEncCtx := gmf.NewCodecCtx(codec)
	if audioEncCtx == nil {
		panic(fmt.Errorf("new output codec context error: %s", err.Error()))
	}
	defer audioEncCtx.Free()

	audioEncCtx.SetSampleFmt(gmf.AV_SAMPLE_FMT_S16P).
		SetSampleRate(44100).
		SetChannels(cc.Channels()).
		SetBitRate(128e3)

	if octx.IsGlobalHeader() {
		audioEncCtx.SetFlag(gmf.CODEC_FLAG_GLOBAL_HEADER)
	}

	audioStream := octx.NewStream(codec)
	if audioStream == nil {
		panic(fmt.Errorf("unable to create stream for audioEnc [%s]", codec.LongName()))
	}
	defer audioStream.Free()

	if err := audioEncCtx.Open(nil); err != nil {
		panic(fmt.Errorf("can't open output codec context: %s", err.Error()))
		return
	}
	audioStream.DumpContexCodec(audioEncCtx)

	/// resample
	options := []*gmf.Option{
		{Key: "in_channel_layout", Val: cc.ChannelLayout()},
		{Key: "out_channel_layout", Val: cc.ChannelLayout()},
		{Key: "in_sample_rate", Val: cc.SampleRate()},
		{Key: "out_sample_rate", Val: audioEncCtx.SampleRate()},
		{Key: "in_sample_fmt", Val: cc.SampleFmt()},
		{Key: "out_sample_fmt", Val: audioEncCtx.SampleFmt()},
	}

	swrCtx, err := gmf.NewSwrCtx(options, audioStream.CodecCtx().Channels(), audioStream.CodecCtx().SampleFmt())
	if err != nil {
		panic(fmt.Errorf("new swr context error: %s", err.Error()))
	}
	if swrCtx == nil {
		panic(fmt.Errorf("unable to create Swr Context"))
	}

	octx.SetStartTime(0)

	if err := octx.WriteHeader(); err != nil {
		panic(err)
	}

	octx.Dump()

	count := 0
	for packet := range ictx.GetNewPackets() {
		srcFrames, err := cc.Decode(packet)
		packet.Free()
		if err != nil {
			log.Printf("capture audio error: %s", err)
			continue
		}

		exit := false
		for _, srcFrame := range srcFrames {
			wrote := fifo.Write(srcFrame)
			count += wrote

			for fifo.SamplesToRead() >= 1152 {
				winFrame := fifo.Read(1152)
				dstFrame, err := swrCtx.Convert(winFrame)
				if err != nil {
					log.Printf("convert audio error: %s", err)
					exit = true
					break
				}
				if dstFrame == nil {
					continue
				}
				winFrame.Free()

				writePacket, err := dstFrame.Encode(audioEncCtx)
				if err != nil {
					log.Println(err)
				}
				if writePacket == nil {
					continue
				}

				if err := octx.WritePacket(writePacket); err != nil {
					log.Printf("write packet err: %s", err.Error())
				}
				writePacket.Free()
				dstFrame.Free()
				if count > int(cc.SampleRate())*10 {
					break
				}
			}
		}
		if exit {
			break
		}
	}

	oavioCtx.Flush()
}

type SourceReader struct {
	reader io.ReadCloser
	buffer []byte
}

func NewSourceReader(reader io.ReadCloser) *SourceReader {
	return &SourceReader{
		reader: reader,
		buffer: make([]byte, gmf.IO_BUFFER_SIZE),
	}
}

func (s *SourceReader) GetAVIOContext(ictx *gmf.FmtCtx) (*gmf.AVIOContext, error) {
	return gmf.NewAVIOContext(
		ictx,
		&gmf.AVIOHandlers{
			ReadPacket: s.Read,
		},
	)
}

func (s *SourceReader) Read() ([]byte, int) {
	if n, err := s.reader.Read(s.buffer); err != nil && err == io.EOF {
		s.reader.Close()
		return s.buffer, n
	} else if err != nil {
		log.Printf("read error: %s", err)
		return nil, n
	} else {
		return s.buffer, n
	}
}

type DestinationWriter struct {
	writer io.Writer
}

func NewDestinationWriter(writer io.Writer) *DestinationWriter {
	return &DestinationWriter{
		writer: writer,
	}
}

func (d *DestinationWriter) GetAVIOContext(ictx *gmf.FmtCtx) (*gmf.AVIOContext, error) {
	return gmf.NewAVIOContext(
		ictx,
		&gmf.AVIOHandlers{
			WritePacket: d.Write,
		},
	)
}

func (d *DestinationWriter) Write(buf []byte) int {
	if n, err := d.writer.Write(buf); err != nil {
		log.Printf("write error: %s", err)
		return 0
	} else {
		return n
	}
}

func assert(i interface{}, err error) interface{} {
	if err != nil {
		panic(err)
	}

	return i
}
