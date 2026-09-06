package service

import (
	"github.com/nawocci/pogu/internal/store"
)

type Service struct {
	Store     *store.Store
	MasterKey []byte
}

func New(st *store.Store, key []byte) *Service {
	return &Service{Store: st, MasterKey: key}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
