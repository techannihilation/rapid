package main

import (
	"encoding/binary"
	"errors"
	"io"
)

type SdpRecord struct {
	Filename string
	MD5      [16]byte
	CRC32    uint32
	Size     uint32
}

func ReadFileRecord(r io.Reader) (*SdpRecord, error) {
	var nameLen uint8

	// Read filename length (1 byte)
	if err := binary.Read(r, binary.LittleEndian, &nameLen); err != nil {
		return nil, err
	}

	// Read filename bytes
	nameBytes := make([]byte, nameLen)
	if _, err := io.ReadFull(r, nameBytes); err != nil {
		return nil, err
	}

	var record SdpRecord
	record.Filename = string(nameBytes)

	// Read MD5 (16 bytes)
	if _, err := io.ReadFull(r, record.MD5[:]); err != nil {
		return nil, err
	}

	// Read CRC32
	if err := binary.Read(r, binary.LittleEndian, &record.CRC32); err != nil {
		return nil, err
	}

	// Read file size
	if err := binary.Read(r, binary.LittleEndian, &record.Size); err != nil {
		return nil, err
	}

	return &record, nil
}

func ReadAllFileRecords(r io.Reader) ([]SdpRecord, error) {
	var records []SdpRecord

	for {
		rec, err := ReadFileRecord(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}

	return records, nil
}

func WriteFileRecord(w io.Writer, record *SdpRecord) error {
	nameLen := len(record.Filename)

	if nameLen > 255 {
		return errors.New("filename too long (max 255 bytes)")
	}

	// Write filename length
	if err := binary.Write(w, binary.LittleEndian, uint8(nameLen)); err != nil {
		return err
	}

	// Write filename bytes
	if _, err := w.Write([]byte(record.Filename)); err != nil {
		return err
	}

	// Write MD5 (16 bytes)
	if _, err := w.Write(record.MD5[:]); err != nil {
		return err
	}

	// Write CRC32
	if err := binary.Write(w, binary.BigEndian, record.CRC32); err != nil {
		return err
	}

	// Write file size
	if err := binary.Write(w, binary.BigEndian, record.Size); err != nil {
		return err
	}

	return nil
}

func WriteAllFileRecords(w io.Writer, records []SdpRecord) error {
	for i := range records {
		if err := WriteFileRecord(w, &records[i]); err != nil {
			return err
		}
	}
	return nil
}
