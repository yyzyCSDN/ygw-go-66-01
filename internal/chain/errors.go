package chain

import (
	"errors"
	"fmt"
)

// 哈希链校验的错误类别。
var (
	ErrLinkBroken   = errors.New("block link does not point at previous block hash")
	ErrHashMismatch = errors.New("block hash does not match its content")
	ErrSequenceGap  = errors.New("block records do not continue previous sequence")
	ErrEmptyChain   = errors.New("chain has no blocks")
)

// ChainError 携带发生错误的块序号，便于定位损坏位置。
type ChainError struct {
	BlockID uint64
	Err     error
}

// Error 实现 error 接口。
func (e *ChainError) Error() string {
	return fmt.Sprintf("chain error at block %d: %v", e.BlockID, e.Err)
}

// Unwrap 暴露底层错误。
func (e *ChainError) Unwrap() error {
	return e.Err
}
