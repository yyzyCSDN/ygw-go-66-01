package model

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Record 表示一条不可变审计记录。Seq 在服务内全局唯一且单调递增，
// WrittenAt 为写入时间，Actor/Action/Detail 描述操作主体、动作与详情。
type Record struct {
	Seq       uint64
	WrittenAt time.Time
	Actor     string
	Action    string
	Detail    string
}

// Validate 校验记录的基本约束：序号必须非零，主体与动作不能为空，
// 写入时间不能为零值。
func (r Record) Validate() error {
	if r.Seq == 0 {
		return errors.New("record seq must be positive")
	}
	if strings.TrimSpace(r.Actor) == "" {
		return errors.New("record actor is required")
	}
	if strings.TrimSpace(r.Action) == "" {
		return errors.New("record action is required")
	}
	if r.WrittenAt.IsZero() {
		return errors.New("record written time is required")
	}
	return nil
}

// Encode 将记录序列化为长度前缀的二进制字节流，供块文件持久化使用。
func (r Record) Encode() []byte {
	actor := []byte(r.Actor)
	action := []byte(r.Action)
	detail := []byte(r.Detail)
	size := 8 + 8 + 4 + len(actor) + 4 + len(action) + 4 + len(detail)
	buf := make([]byte, size)
	off := 0
	binary.BigEndian.PutUint64(buf[off:], r.Seq)
	off += 8
	binary.BigEndian.PutUint64(buf[off:], uint64(r.WrittenAt.UnixNano()))
	off += 8
	writeBytes(buf, &off, actor)
	writeBytes(buf, &off, action)
	writeBytes(buf, &off, detail)
	return buf
}

// DecodeRecord 从二进制字节流还原一条记录。
func DecodeRecord(data []byte) (Record, error) {
	if len(data) < 32 {
		return Record{}, fmt.Errorf("record payload too short: %d", len(data))
	}
	rec := Record{}
	rec.Seq = binary.BigEndian.Uint64(data[0:8])
	rec.WrittenAt = time.Unix(0, int64(binary.BigEndian.Uint64(data[8:16]))).UTC()
	off := 16
	var err error
	if rec.Actor, err = readBytes(data, &off); err != nil {
		return Record{}, err
	}
	if rec.Action, err = readBytes(data, &off); err != nil {
		return Record{}, err
	}
	if rec.Detail, err = readBytes(data, &off); err != nil {
		return Record{}, err
	}
	if off != len(data) {
		return Record{}, fmt.Errorf("record payload has %d trailing bytes", len(data)-off)
	}
	return rec, nil
}

// Keywords 提取记录详情中的检索关键词，供索引组件使用。
func (r Record) Keywords() []string {
	var words []string
	for _, part := range strings.FieldsFunc(r.Detail, func(c rune) bool {
		return c == ' ' || c == ',' || c == ';' || c == ':' || c == '|'
	}) {
		part = strings.Trim(part, "[]{}()")
		if part != "" {
			words = append(words, strings.ToLower(part))
		}
	}
	return words
}

func writeBytes(buf []byte, off *int, value []byte) {
	binary.BigEndian.PutUint32(buf[*off:], uint32(len(value)))
	*off += 4
	copy(buf[*off:], value)
	*off += len(value)
}

func readBytes(data []byte, off *int) (string, error) {
	if *off+4 > len(data) {
		return "", fmt.Errorf("record payload truncated at byte %d", *off)
	}
	size := int(binary.BigEndian.Uint32(data[*off:]))
	*off += 4
	if *off+size > len(data) {
		return "", fmt.Errorf("record payload field exceeds buffer at byte %d", *off)
	}
	value := string(data[*off : *off+size])
	*off += size
	return value, nil
}
