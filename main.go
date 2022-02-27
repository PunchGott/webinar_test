package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
)

const (
	// BoxHeaderSize Size of box header.
	BoxHeaderSize = int64(8)
)

var globalMap map[uint32]uint32

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

type MoovContainer struct {
	*Box
	Mvhd *MvhdBox
	Trak *TrackBox
}

func (b *MoovContainer) parse() error {
	boxes := readBoxes(b.Reader, b.Start+BoxHeaderSize, b.Size-BoxHeaderSize)

	count := 0
	for _, box := range boxes {
		switch box.Name {
		case "mvhd":
			b.Mvhd = &MvhdBox{Box: box}
			b.Mvhd.parse()
		case "trak":
			if count == 0 {
				b.Trak = &TrackBox{Box: box}
				b.Trak.parse()
				count++
			}

		}
	}

	return nil
}

type MvhdBox struct {
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

func (b *MvhdBox) parse() error {
	data := b.ReadBoxData()
	b.Version = data[0]
	b.Timescale = binary.BigEndian.Uint32(data[12:16])
	b.Duration = binary.BigEndian.Uint32(data[16:20])
	b.Rate = fixed32(data[20:24])
	b.Volume = fixed16(data[24:26])
	return nil
}

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

type MediaBox struct {
	*Box
	Mdhd *MediaHeaderBox
	Hdir *HandlerBox
	Minf *MediaInformationBox
}

func (b *MediaBox) parse() error {
	boxes := readBoxes(b.Reader, b.Start+BoxHeaderSize, b.Size-BoxHeaderSize)

	for _, box := range boxes {
		switch box.Name {
		case "mdhd":
			b.Mdhd = &MediaHeaderBox{Box: box}
			b.Mdhd.parse()
		case "minf":
			b.Minf = &MediaInformationBox{Box: box}
			b.Minf.parse()
		}
	}
	return nil
}

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

type HandlerBox struct {
	*Box
	Version     uint8
	Flags       [3]byte
	PreDefined  uint32
	HandlerType uint32
	Name        string
}

func (b *HandlerBox) parse() error {

	return nil
}

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

type VideoMediaHeaderBox struct {
	*Box
	Version uint8
	Flags   [3]byte
}

func (b *VideoMediaHeaderBox) parse() error {

	return nil
}

type SoundMediaHeaderBox struct {
	*Box
	Version uint8
	Flags   [3]byte
}

func (b *SoundMediaHeaderBox) parse() error {

	return nil
}

type HintMediaHeaderBox struct {
	*Box
	Version uint8
	Flags   [3]byte
}

func (b *HintMediaHeaderBox) parse() error {

	return nil
}

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
		fmt.Println("stsz.entry_size: ", binary.BigEndian.Uint32(data[4*(i+2):4*(i+2)+4]))
	}

	return nil
}

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
		fmt.Println("stsc.first_chunk: ", binary.BigEndian.Uint32(data[4*(i+1):4*(i+1)+4]))
		fmt.Println("stsc.samples_per_chunk: ", binary.BigEndian.Uint32(data[4*(i+1):4*(i+1)+4]))
		fmt.Println("stsc.sample_description_chunk: ", binary.BigEndian.Uint32(data[4*(i+1):4*(i+1)+4]))
	}
	return nil
}

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
		fmt.Println("stco.chunk_offset: ", binary.BigEndian.Uint32(data[4*(i+1):4*(i+1)+4]))
	}

	globalMap = make(map[uint32]uint32)
	return nil
}

type MdatContainer struct {
	*Box
	Data []byte
}

func (b *MdatContainer) parse() error {
	b.Data = b.ReadBoxData()
	return nil
}

// Mp4Reader defines an mp4 reader structure.
type Mp4Reader struct {
	Reader io.ReaderAt
	Ftyp   *FtypBox
	Moov   *MoovContainer
	Mdat   *MdatContainer
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
			m.Moov = &MoovContainer{Box: box}
			m.Moov.parse()

		case "mdat":
			m.Mdat = &MdatContainer{Box: box}
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

func main() {
	if len(os.Args) < 2 {
		fmt.Println("missing argument, provide an mp4 file!")
		return
	}

	mp4, err := Open(os.Args[1])
	if err != nil {
		fmt.Println("unable to open file")
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
	// fmt.Println("moov.Trak.Tkhd.Flags: ", mp4.Moov.Trak.Tkhd.Flags)
	fmt.Println("moov.Trak.Tkhd.CreationTime: ", mp4.Moov.Trak.Tkhd.CreationTime)
	fmt.Println("moov.Trak.Tkhd.ModificationTime: ", mp4.Moov.Trak.Tkhd.ModificationTime)
	fmt.Println("moov.Trak.Tkhd.Duration: ", mp4.Moov.Trak.Tkhd.Duration)
	fmt.Println("moov.Trak.Tkhd.TrackID: ", mp4.Moov.Trak.Tkhd.TrackID)
	fmt.Println("moov.Trak.Tkhd.Volume: ", mp4.Moov.Trak.Tkhd.Volume)
	fmt.Printf("moov.Trak.Tkhd.Width: %f \n", mp4.Moov.Trak.Tkhd.Width)
	fmt.Printf("moov.Trak.Tkhd.Height: %f \n", mp4.Moov.Trak.Tkhd.Height)

	// for i := 0; i < 5; i++ {
	//	fmt.Println("mdata: ", strconv.FormatUint(mp4.Mdat.Data[i], 16))
	// }
	fmt.Println("mdata: ", mp4.Mdat.Data[0:16], mp4.Mdat.Data[4:16])

	messageReturn := bytes.NewBuffer([]byte{0, 0, 0, 1})
	messageReturn.Write(mp4.Mdat.Data[4:])

	fmt.Println("mp4 offset: ", mp4.ReadBytesAt(20, 48))

	permissions := 0644 // or whatever you need
	err2 := ioutil.WriteFile("file.txt", mp4.Mdat.Data[4:16], os.FileMode(permissions))
	if err2 != nil {
		fmt.Println("unable to open file")
	}
}
