package chain

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"auditlog/internal/model"
)

// Chain 维护已知块集合与块间链接关系，负责新块链接与完整性校验。
type Chain struct {
	mu    sync.RWMutex
	known map[uint64]struct{}
}

// NewChain 创建空哈希链。
func NewChain() *Chain {
	return &Chain{known: make(map[uint64]struct{})}
}

// Register 登记一个已落盘的块序号。
func (c *Chain) Register(id uint64) {
	c.mu.Lock()
	c.known[id] = struct{}{}
	c.mu.Unlock()
}

// Knows 判断块序号是否已登记。
func (c *Chain) Knows(id uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.known[id]
	return ok
}

// Link 将新块 cur 链接到前一块 prev：计算 prev 的块哈希写入 cur.PrevHash，
// 再计算 cur 自身的块哈希。prev 为空表示这是链上的第一块。
func (c *Chain) Link(prev, cur *model.Block) error {
	if cur == nil {
		return errors.New("link target block is nil")
	}
	if prev != nil && len(prev.Records) > 0 && len(cur.Records) > 0 {
		expected := prev.Records[len(prev.Records)-1].Seq + 1
		if cur.Records[0].Seq != expected {
			return &ChainError{
				BlockID: cur.ID,
				Err:     fmt.Errorf("%w: expected seq %d, got %d", ErrSequenceGap, expected, cur.Records[0].Seq),
			}
		}
	}
	if prev == nil {
		cur.PrevHash = 0
		cur.Hash = BlockHash(cur)
		if cur.Hash == 0 {
			return &ChainError{BlockID: cur.ID, Err: errors.New("genesis block hash is zero")}
		}
		c.Register(cur.ID)
		return nil
	}
	prevHash := BlockHash(prev)
	cur.PrevHash = prevHash
	cur.Hash = BlockHash(cur)
	if cur.Hash == 0 {
		return &ChainError{BlockID: cur.ID, Err: errors.New("block hash is zero")}
	}
	c.Register(cur.ID)
	return nil
}

// LinkOrRollback 尝试把新块链接到前一块；链接失败时回滚已登记状态并返回错误，
// 供追加写入在失败时保持链状态一致。
func (c *Chain) LinkOrRollback(prev, cur *model.Block) error {
	if err := c.Link(prev, cur); err != nil {
		c.mu.Lock()
		delete(c.known, cur.ID)
		c.mu.Unlock()
		return err
	}
	return nil
}

// RefreshHash 重新计算块的自身哈希，用于块内记录追加后维持链完整性。
// 块的 PrevHash 保持不变，只有块内容变化时调用。
func (c *Chain) RefreshHash(block *model.Block) error {
	if block == nil {
		return errors.New("refresh target block is nil")
	}
	block.Hash = BlockHash(block)
	return nil
}

// VerifyLink 校验 prev 与 cur 的链接关系以及 cur 的自身哈希。
func (c *Chain) VerifyLink(prev, cur *model.Block) error {
	if cur == nil {
		return &ChainError{BlockID: 0, Err: errors.New("verify target block is nil")}
	}
	if prev != nil {
		want := BlockHash(prev)
		if cur.PrevHash != want {
			return &ChainError{BlockID: cur.ID, Err: ErrLinkBroken}
		}
	}
	if cur.Hash != BlockHash(cur) {
		return &ChainError{BlockID: cur.ID, Err: ErrHashMismatch}
	}
	return nil
}

// VerifyAll 按序号遍历全部块并逐对校验链接，返回全部断链位置。
func (c *Chain) VerifyAll(blocks []*model.Block) ([]*model.Block, []error, error) {
	if len(blocks) == 0 {
		return nil, nil, ErrEmptyChain
	}
	sorted := make([]*model.Block, len(blocks))
	copy(sorted, blocks)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	var (
		previous *model.Block
		verified []*model.Block
		broken   []error
	)
	for _, block := range sorted {
		if err := c.VerifyLink(previous, block); err != nil {
			broken = append(broken, err)
			continue
		}
		verified = append(verified, block)
		previous = block
	}
	return verified, broken, nil
}
