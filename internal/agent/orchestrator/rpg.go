package orchestrator

import (
	"context"
	"errors"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
)

// online 报告当前是否配置了可用的 AI。
func (s *Session) online() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ai.Online()
}

// Codex 返回图鉴。
func (s *Session) Codex() (dto.CodexV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return dto.CodexV1{}, err
	}
	return g.q.Codex(st), nil
}

// Card 返回单张介绍卡（物品 / 装备 / 技能 / 敌人 / 势力 / 地点 / 图鉴条目）。
func (s *Session) Card(id string) (dto.CardV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return dto.CardV1{}, err
	}
	c, _ := g.q.Card(st, id)
	return c, nil
}

// Relations 返回玩家视角的关系网。
func (s *Session) Relations() (dto.RelationsV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return dto.RelationsV1{}, err
	}
	return g.q.Relations(st), nil
}

// Cards 返回角色卡（在场 / 归档 / 死亡）。
func (s *Session) Cards() (dto.CardsV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return dto.CardsV1{}, err
	}
	return g.q.Cards(st), nil
}

// Growth 返回成长面板（等级 / 属性点 / 装备 / 技能）。
func (s *Session) Growth() (*dto.GrowthV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return nil, err
	}
	return g.q.Growth(st), nil
}

// Combat 返回当前战斗面板（不在战斗中为 nil）。
func (s *Session) Combat() (*dto.CombatV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return nil, err
	}
	return g.q.Combat(st), nil
}

// Portrait 返回角色立绘（没有时 ok=false，UI 显示占位头像）。
func (s *Session) Portrait(_ context.Context, id string) (dto.PortraitV1, bool, error) {
	_, _, g, err := s.current()
	if err != nil {
		return dto.PortraitV1{}, false, err
	}
	v, ok := g.q.Portrait(id)
	return v, ok, nil
}

// Mechs 返回机械甲胄卡列表。
func (s *Session) Mechs() (dto.MechsV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return dto.MechsV1{}, err
	}
	return g.q.Mechs(st), nil
}

// Mech 返回一张机械甲胄卡。
func (s *Session) Mech(id string) (dto.MechCardV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return dto.MechCardV1{}, err
	}
	c, ok := g.q.Mech(st, id)
	if !ok {
		return c, errors.New("没有这台机甲")
	}
	return c, nil
}
