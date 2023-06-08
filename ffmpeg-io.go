package main

import (
	"io"
	"log"

	gmf "github.com/3d0c/gmf"
)

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
