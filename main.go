package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
)

const (
	// BoxHeaderSize Size of box header.
	BoxHeaderSize = int64(8)
)

// Fixed16 is an 8.8 Fixed Point Decimal notation
type Fixed16 uint16

func (f Fixed16) String() string {
	return fmt.Sprintf("%v", uint16(f)>>8)
}

func fixed16(bytes []byte) Fixed16 {
	return Fixed16(binary.BigEndian.Uint16(bytes))
}

// Fixed32 is a 16.16 Fixed Point Decimal notation
type Fixed32 uint32

func fixed32(bytes []byte) Fixed32 {
	return Fixed32(binary.BigEndian.Uint32(bytes))
}

// Mp4Reader defines an mp4 reader structure.
type Mp4Reader struct {
	Reader io.ReaderAt
	Ftyp   *FtypBox
	Moov   *MovieBox
	Mdat   *MediaDataBox
	Size   int64
}

// Parse reads an MP4 reader for atom boxes.
func (m *Mp4Reader) Parse() error {
	if m.Size == 0 {
		if ofile, ok := m.Reader.(*os.File); ok {
			info, err := ofile.Stat()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return err
			}
			m.Size = info.Size()
		}
	}

	boxes := readBoxes(m, int64(0), m.Size)
	for _, box := range boxes {
		switch box.Name {
		case "ftyp":
			m.Ftyp = &FtypBox{Box: box}
			m.Ftyp.parse()

		case "moov":
			m.Moov = &MovieBox{Box: box}
			m.Moov.parse()

		case "mdat":
			m.Mdat = &MediaDataBox{Box: box}
			m.Mdat.parse()
		}
	}
	return nil
}

// ReadBoxAt reads a box from an offset.
func (m *Mp4Reader) ReadBoxAt(offset int64) (boxSize uint32, boxType string) {
	buf := m.ReadBytesAt(BoxHeaderSize, offset)
	boxSize = binary.BigEndian.Uint32(buf[0:4])
	boxType = string(buf[4:8])
	return boxSize, boxType
}

// ReadBytesAt reads a box at n and offset.
func (m *Mp4Reader) ReadBytesAt(n int64, offset int64) (word []byte) {
	buf := make([]byte, n)
	if _, error := m.Reader.ReadAt(buf, offset); error != nil {
		fmt.Println(error)
		return
	}
	return buf
}

func readBoxes(m *Mp4Reader, start int64, n int64) (l []*Box) {
	for offset := start; offset < start+n; {
		size, name := m.ReadBoxAt(offset)

		b := &Box{
			Name:   string(name),
			Size:   int64(size),
			Reader: m,
			Start:  offset,
		}

		l = append(l, b)
		offset += int64(size)
	}
	return l
}

// Open opens a file and returns an &Mp4Reader{}.
func Open(path string) (f *Mp4Reader, err error) {
	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, err
	}

	f = &Mp4Reader{
		Reader: file,
	}
	return f, f.Parse()
}

// Box defines an Atom Box structure.
type Box struct {
	Name        string
	Size, Start int64
	Reader      *Mp4Reader
}

// ReadBoxData reads the box data from an atom box.
func (b *Box) ReadBoxData() []byte {
	if b.Size <= BoxHeaderSize {
		return nil
	}
	return b.Reader.ReadBytesAt(b.Size-BoxHeaderSize, b.Start+BoxHeaderSize)
}

// FtypBox - File Type Box
// Box Type: ftyp
// Container: File
// Mandatory: Yes
// Quantity: Exactly one
type FtypBox struct {
	*Box
	MajorBrand       string   // Brand identifer.
	MinorVersion     uint32   // Informative integer for the minor version of the major brand.
	CompatibleBrands []string // A list, to the end of the box, of brands.
}

func (b *FtypBox) parse() error {
	data := b.ReadBoxData()
	b.MajorBrand = string(data[0:4])
	b.MinorVersion = binary.BigEndian.Uint32(data[4:8])
	if len(data) > 8 {
		for i := 8; i < len(data); i += 4 {
			b.CompatibleBrands = append(b.CompatibleBrands, string(data[i:i+4]))
		}
	}
	return nil
}

// MovieBox - The metadata for a presentation is stored in the single Movie Box
// Box Type: ‘moov’
// Container: File
// Mandatory: Yes
// Quantity: Exactly one
type MovieBox struct {
	*Box
	Mvhd *MovieHeaderBox
	// @todo На самом деле их можеть быть сколь угодно много, так что по-хорошему тут должен быть массив
	Trak *TrackBox
}

func (b *MovieBox) parse() error {
	boxes := readBoxes(b.Reader, b.Start+BoxHeaderSize, b.Size-BoxHeaderSize)

	for _, box := range boxes {
		switch box.Name {
		case "mvhd":
			b.Mvhd = &MovieHeaderBox{Box: box}
			b.Mvhd.parse()
		case "trak":
			if trak := parseTrack(box); trak.Mdia.Hdlr.TypeName == "vide" {
				b.Trak = trak
			}

		}
	}

	return nil
}

func parseTrack(box *Box) *TrackBox {
	trackBox := &TrackBox{Box: box}
	trackBox.parse()
	return trackBox
}

// MovieHeaderBox - This box defines overall information which is media-independent
// Box Type: ‘mvhd’
// Container: Movie Box (‘moov’)
// Mandatory: Yes
// Quantity: Exactly one
type MovieHeaderBox struct {
	*Box
	Version          uint8
	Flags            uint32
	CreationTime     uint32
	ModificationTime uint32
	Timescale        uint32
	Duration         uint32
	Rate             Fixed32
	Volume           Fixed16
}

func (b *MovieHeaderBox) parse() error {
	data := b.ReadBoxData()
	b.Version = data[0]
	b.Timescale = binary.BigEndian.Uint32(data[12:16])
	b.Duration = binary.BigEndian.Uint32(data[16:20])
	b.Rate = fixed32(data[20:24])
	b.Volume = fixed16(data[24:26])
	return nil
}

// TrackBox - This is a container box for a single track of a presentation
// Box Type: ‘trak’
// Container: Movie Box (‘moov’)
// Mandatory: Yes
// Quantity: One or more
type TrackBox struct {
	*Box
	Tkhd *TrackHeaderBox
	Mdia *MediaBox
}

func (b *TrackBox) parse() error {
	boxes := readBoxes(b.Reader, b.Start+BoxHeaderSize, b.Size-BoxHeaderSize)

	for _, box := range boxes {
		switch box.Name {
		case "tkhd":
			b.Tkhd = &TrackHeaderBox{Box: box}
			b.Tkhd.parse()

		case "mdia":
			b.Mdia = &MediaBox{Box: box}
			b.Mdia.parse()
		}
	}
	return nil
}

// TrackHeaderBox - This box specifies the characteristics of a single track
// Box Type: ‘tkhd’
// Container: Track Box (‘trak’)
// Mandatory: Yes
// Quantity: Exactly one
type TrackHeaderBox struct {
	*Box
	Version          uint8
	Flags            [3]byte
	CreationTime     uint32
	ModificationTime uint32
	TrackID          uint32
	Reserved         uint32
	Duration         uint32
	Layer            uint16
	AlternateGroup   uint16
	Volume           Fixed16
	Width            Fixed16
	Height           Fixed16
}

func (b *TrackHeaderBox) parse() error {
	fmt.Println("tkhd.parse()")
	data := b.ReadBoxData()
	b.Version = data[0]
	for i := 0; i < 3; i++ {
		b.Flags[i] = data[i+1]
	}
	// flags 24 bit
	b.CreationTime = binary.BigEndian.Uint32(data[4:8])
	b.ModificationTime = binary.BigEndian.Uint32(data[8:12])
	b.TrackID = binary.BigEndian.Uint32(data[12:16])
	b.Reserved = binary.BigEndian.Uint32(data[16:20])
	b.Duration = binary.BigEndian.Uint32(data[20:24])
	// reserved [2]uint32
	b.Layer = binary.BigEndian.Uint16(data[32:34])
	b.AlternateGroup = binary.BigEndian.Uint16(data[34:36])
	b.Volume = fixed16(data[36:38])
	// reserved uint16 [38:40]
	// matrix [9]int32 [40:76]
	b.Width = fixed16(data[76:80])
	b.Height = fixed16(data[80:84])

	return nil
}

// MediaBox - The media declaration container contains all the objects that declare information about the media data within a track
// Box Type: ‘mdia’
// Container: Track Box (‘trak’)
// Mandatory: Yes
// Quantity: Exactly one
type MediaBox struct {
	*Box
	Mdhd *MediaHeaderBox
	Hdlr *HandlerBox
	Minf *MediaInformationBox
}

func (b *MediaBox) parse() error {
	fmt.Println("MediaBox.parse()")
	boxes := readBoxes(b.Reader, b.Start+BoxHeaderSize, b.Size-BoxHeaderSize)

	for _, box := range boxes {
		switch box.Name {
		case "mdhd":
			b.Mdhd = &MediaHeaderBox{Box: box}
			b.Mdhd.parse()

		case "hdlr":
			b.Hdlr = &HandlerBox{Box: box}
			b.Hdlr.parse()

		case "minf":
			b.Minf = &MediaInformationBox{Box: box}
			b.Minf.parse()
		}
	}
	return nil
}

// MediaHeaderBox - The media header declares overall information that is media-independent
// Box Type: ‘mdhd’
// Container: Media Box (‘mdia’)
// Mandatory: Yes
// Quantity: Exactly one
type MediaHeaderBox struct {
	*Box
	Version          uint8
	Flags            [3]byte
	CreationTime     uint32
	ModificationTime uint32
	Timescale        uint32
	Duration         uint32
	Language         [3]byte
	PreDefined       uint16
}

func (b *MediaHeaderBox) parse() error {
	fmt.Println("mdhd.parse()")
	data := b.ReadBoxData()
	b.Version = data[0]
	for i := 0; i < 3; i++ {
		b.Flags[i] = data[i+1]
	}
	// flags 24 bit
	b.CreationTime = binary.BigEndian.Uint32(data[4:8])
	b.ModificationTime = binary.BigEndian.Uint32(data[8:12])
	b.Timescale = binary.BigEndian.Uint32(data[8:12])
	b.Duration = binary.BigEndian.Uint32(data[12:16])
	// b.Language = language(data[16:19])
	b.PreDefined = binary.BigEndian.Uint16(data[19:21])
	return nil
}

// Handler Reference Box - This box within a Media Box declares the process by which the media-data in the track is presented
// Box Type: ‘hdlr’
// Container: Media Box (‘mdia’) or Meta Box (‘meta’)
// Mandatory: Yes
// Quantity: Exactly one
type HandlerBox struct {
	*Box
	Version     uint8
	Flags       [3]byte
	PreDefined  uint32
	HandlerType uint32
	Reserved    [3]uint32
	TypeName    string
}

func (b *HandlerBox) parse() error {
	fmt.Println("handlerType.parse()")
	data := b.ReadBoxData()
	b.Version = data[0]
	for i := 0; i < 3; i++ {
		b.Flags[i] = data[i+1]
	}
	// flags 24 bit
	b.PreDefined = binary.BigEndian.Uint32(data[4:8])
	b.HandlerType = binary.BigEndian.Uint32(data[8:12])
	// b.reserved = reserverd(data[12:24])
	b.TypeName = string(data[8:12])

	return nil
}

// MediaInformationBox - This box contains all the objects that declare characteristic information of the media in the track.
// Box Type: ‘minf’
// Container: Media Box (‘mdia’)
// Mandatory: Yes
// Quantity: Exactly one
type MediaInformationBox struct {
	*Box
	Vmhd *VideoMediaHeaderBox
	Smhd *SoundMediaHeaderBox
	Hmhd *HintMediaHeaderBox
	// Nmhd *NullMediaHeaderBox
	// Dinf *DataInformationBox
	Stbl *SampleTableBox
}

func (b *MediaInformationBox) parse() error {
	boxes := readBoxes(b.Reader, b.Start+BoxHeaderSize, b.Size-BoxHeaderSize)

	for _, box := range boxes {
		switch box.Name {
		case "vmhd":
			b.Vmhd = &VideoMediaHeaderBox{Box: box}
			b.Vmhd.parse()
		case "smhd":
			b.Smhd = &SoundMediaHeaderBox{Box: box}
			b.Smhd.parse()
		case "hmhd":
			b.Hmhd = &HintMediaHeaderBox{Box: box}
			b.Hmhd.parse()
		case "stbl":
			b.Stbl = &SampleTableBox{Box: box}
			b.Stbl.parse()
		}
	}
	return nil
}

// Video Media Header Box - The video media header contains general presentation information, independent of the coding, for video media
type VideoMediaHeaderBox struct {
	*Box
	Version      uint8
	Flags        [3]byte
	GraphicsMode uint16
	OpColor      [3]uint16
}

func (b *VideoMediaHeaderBox) parse() error {

	return nil
}

// SoundMediaHeaderBox - The sound media header contains general presentation information, independent of the coding, for audio media
type SoundMediaHeaderBox struct {
	*Box
	Version  uint8
	Flags    [3]byte
	Balance  uint16
	Reserved uint16
}

func (b *SoundMediaHeaderBox) parse() error {

	return nil
}

// HintMediaHeaderBox - The hint media header contains general information, independent of the protocol, for hint tracks
type HintMediaHeaderBox struct {
	*Box
	Version uint8
	Flags   [3]byte
}

func (b *HintMediaHeaderBox) parse() error {

	return nil
}

// SampleTableBox - The sample table contains all the time and data indexing of the media samples in a track
// Box Type: ‘stbl’
// Container: Media Information Box (‘minf’)
// Mandatory: Yes
// Quantity: Exactly one
type SampleTableBox struct {
	*Box
	Stsz *SampleSizeBox
	Stsc *SampleToChunkBox
	Stco *ChunkOffsetBox
}

func (b *SampleTableBox) parse() error {
	boxes := readBoxes(b.Reader, b.Start+BoxHeaderSize, b.Size-BoxHeaderSize)

	for _, box := range boxes {
		switch box.Name {
		case "stsz":
			b.Stsz = &SampleSizeBox{Box: box}
			b.Stsz.parse()
		case "stsc":
			b.Stsc = &SampleToChunkBox{Box: box}
			b.Stsc.parse()
		case "stco":
			b.Stco = &ChunkOffsetBox{Box: box}
			b.Stco.parse()
		}
	}
	return nil
}

// SampleSizeBox - This box contains the sample count and a table giving the size in bytes of each sample
// Box Type: stsz’, ‘stz2’
// Container: Sample Table Box (‘stbl’)
// Mandatory: Yes
// Quantity: Exactly one variant must be present
type SampleSizeBox struct {
	*Box
	Version     uint8
	Flags       [3]byte
	SampleSize  uint32
	SampleCount uint32
}

func (b *SampleSizeBox) parse() error {
	fmt.Println("SampleSizeBox")
	data := b.ReadBoxData()
	b.Version = data[0]
	for i := 0; i < 3; i++ {
		b.Flags[i] = data[i+1]
	}

	b.SampleSize = binary.BigEndian.Uint32(data[4:8])
	b.SampleCount = binary.BigEndian.Uint32(data[8:12])
	fmt.Println("stsz.SampleSize: ", b.SampleSize)
	fmt.Println("stsz.SampleCount: ", b.SampleCount)
	for i := uint32(1); i <= b.SampleCount; i++ {
		//fmt.Println("stsz.entry_size: ", binary.BigEndian.Uint32(data[4*(i+2):4*(i+2)+4]))
	}

	return nil
}

// SampleToChunkBox - Samples within the media data are grouped into chunks. Chunks can be of different sizes, and the samples
// within a chunk can have different sizes
// Box Type: ‘stsc’
//Container: Sample Table Box (‘stbl’)
//Mandatory: Yes
//Quantity: Exactly one
type SampleToChunkBox struct {
	*Box
	Version    uint8
	Flags      [3]byte
	EntryCount uint32
}

func (b *SampleToChunkBox) parse() error {
	fmt.Println("SampleToChunkBox")
	data := b.ReadBoxData()
	b.Version = data[0]
	for i := 0; i < 3; i++ {
		b.Flags[i] = data[i+1]
	}
	b.EntryCount = binary.BigEndian.Uint32(data[4:8])
	for i := uint32(1); i <= b.EntryCount; i++ {
		//fmt.Println("stsc.first_chunk: ", binary.BigEndian.Uint32(data[4*(i+1):4*(i+1)+4]))
		//fmt.Println("stsc.samples_per_chunk: ", binary.BigEndian.Uint32(data[4*(i+1):4*(i+1)+4]))
		//fmt.Println("stsc.sample_description_chunk: ", binary.BigEndian.Uint32(data[4*(i+1):4*(i+1)+4]))
	}
	return nil
}

// ChunkOffsetBox - The chunk offset table gives the index of each chunk into the containing file
// Box Type: ‘stco’, ‘co64’
// Container: Sample Table Box (‘stbl’)
// Mandatory: Yes
// Quantity: Exactly one variant must be present
type ChunkOffsetBox struct {
	*Box
	Version    uint8
	Flags      [3]byte
	EntryCount uint32
}

func (b *ChunkOffsetBox) parse() error {
	fmt.Println("ChunkOffsetBox")
	data := b.ReadBoxData()
	b.Version = data[0]
	for i := 0; i < 3; i++ {
		b.Flags[i] = data[i+1]
	}
	b.EntryCount = binary.BigEndian.Uint32(data[4:8])
	fmt.Println("stco.EntryCount: ", b.EntryCount)
	for i := uint32(1); i <= b.EntryCount; i++ {
		//fmt.Println("stco.chunk_offset: ", binary.BigEndian.Uint32(data[4*(i+1):4*(i+1)+4]))
	}

	return nil
}

// MediaDataBox - This box contains the media data
// Box Type: ‘mdat’
// Container: File
// Mandatory: No
// Quantity: Any number
type MediaDataBox struct {
	*Box
	Data []byte
}

func (b *MediaDataBox) parse() error {
	b.Data = b.ReadBoxData()
	return nil
}

func extractVideoChunks(mp4 *Mp4Reader) (videoStream []byte) {
	chunks := bytes.NewBuffer([]byte{0, 0, 0, 1})
	chunks.Write(mp4.Mdat.Data[4:])

	// @todo Extract videChunks, build full video stream and convert in Annex-B format
	// ...

	return chunks.Bytes()
}

func writeVideoStreamInAnnexBFormat(bytes []byte, fileName string) error {
	err := ioutil.WriteFile(fileName, bytes, os.FileMode(0644))
	if err != nil {
		fmt.Println("Unable to open file")
		return err
	}
	return nil
}

func main() {
	inputFileName := flag.String("input", "input.mp4", "name of .mp4 file")
	outputFileName := flag.String("output", "output.h264", "name of output file")
	flag.Parse()

	mp4, err := Open(*inputFileName)
	defer mp4.Reader.(*os.File).Close()
	if err != nil {
		fmt.Println("Unable to open file")
		return
	}

	fmt.Println("ftyp.name: ", mp4.Ftyp.Name)
	fmt.Println("ftyp.major_brand: ", mp4.Ftyp.MajorBrand)
	fmt.Println("ftyp.minor_version: ", mp4.Ftyp.MinorVersion)
	fmt.Println("ftyp.compatible_brands: ", mp4.Ftyp.CompatibleBrands)

	fmt.Println("moov.name: ", mp4.Moov.Name, mp4.Moov.Size)
	fmt.Println("moov.mvhd.name: ", mp4.Moov.Mvhd.Name)
	fmt.Println("moov.mvhd.version: ", mp4.Moov.Mvhd.Version)
	fmt.Println("moov.mvhd.volume: ", mp4.Moov.Mvhd.Volume)

	fmt.Println("moov.Trak.Tkhd.Version: ", mp4.Moov.Trak.Tkhd.Version)
	fmt.Println("moov.Trak.Tkhd.CreationTime: ", mp4.Moov.Trak.Tkhd.CreationTime)
	fmt.Println("moov.Trak.Tkhd.ModificationTime: ", mp4.Moov.Trak.Tkhd.ModificationTime)
	fmt.Println("moov.Trak.Tkhd.Duration: ", mp4.Moov.Trak.Tkhd.Duration)
	fmt.Println("moov.Trak.Tkhd.TrackID: ", mp4.Moov.Trak.Tkhd.TrackID)
	fmt.Println("moov.Trak.Tkhd.Volume: ", mp4.Moov.Trak.Tkhd.Volume)
	fmt.Printf("moov.Trak.Tkhd.Width: %f \n", mp4.Moov.Trak.Tkhd.Width)
	fmt.Printf("moov.Trak.Tkhd.Height: %f \n", mp4.Moov.Trak.Tkhd.Height)

	fmt.Println("moov.Trak.Mdia.Hdir.TypeName: ", mp4.Moov.Trak.Mdia.Hdlr.TypeName)

	writeVideoStreamInAnnexBFormat(extractVideoChunks(mp4), *outputFileName)
}
