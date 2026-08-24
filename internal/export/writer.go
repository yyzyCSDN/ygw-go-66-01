package export

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"auditlog/internal/model"
)

// FormatExport 把记录格式化为导出行：seq,时间,主体,动作,详情。
func FormatExport(records []model.Record) []byte {
	lines := make([]string, 0, len(records))
	for _, rec := range records {
		lines = append(lines, FormatLine(rec))
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

// FormatLine 格式化单条导出行。
func FormatLine(rec model.Record) string {
	at := rec.WrittenAt.UTC().Format(time.RFC3339Nano)
	actor := escapeField(rec.Actor)
	action := escapeField(rec.Action)
	detail := escapeField(rec.Detail)
	return fmt.Sprintf("%d,%s,%s,%s,%s", rec.Seq, at, actor, action, detail)
}

// ParseExportLine 解析一条导出行，供校验与断点续传使用。
func ParseExportLine(line string) (model.Record, error) {
	fields := splitLine(line)
	if len(fields) != 5 {
		return model.Record{}, fmt.Errorf("export line must have 5 fields, got %d", len(fields))
	}
	seq, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return model.Record{}, fmt.Errorf("invalid export seq %q", fields[0])
	}
	at, err := time.Parse(time.RFC3339Nano, fields[1])
	if err != nil {
		return model.Record{}, fmt.Errorf("invalid export time %q", fields[1])
	}
	return model.Record{
		Seq:       seq,
		WrittenAt: at,
		Actor:     unescapeField(fields[2]),
		Action:    unescapeField(fields[3]),
		Detail:    unescapeField(fields[4]),
	}, nil
}

func escapeField(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, ",", `\,`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

func unescapeField(value string) string {
	value = strings.ReplaceAll(value, `\n`, "\n")
	value = strings.ReplaceAll(value, `\,`, ",")
	value = strings.ReplaceAll(value, `\\`, `\`)
	return value
}

func splitLine(line string) []string {
	fields := make([]string, 0, 5)
	var builder strings.Builder
	escaped := false
	for _, ch := range line {
		if escaped {
			builder.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == ',' {
			fields = append(fields, builder.String())
			builder.Reset()
			continue
		}
		builder.WriteRune(ch)
	}
	fields = append(fields, builder.String())
	return fields
}
