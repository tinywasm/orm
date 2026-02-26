package tests

import (
	"github.com/tinywasm/orm"
)

// MockAdapter is a mock implementation of the Adapter interface.
type MockAdapter struct {
	LastQuery orm.Query
	LastModel orm.Model
	LastFactory func() orm.Model
	LastEach    func(orm.Model)
	ReturnErr   error
}

func (m *MockAdapter) Execute(q orm.Query, model orm.Model, factory func() orm.Model, each func(orm.Model)) error {
	m.LastQuery = q
	m.LastModel = model
	m.LastFactory = factory
	m.LastEach = each
	return m.ReturnErr
}

// MockModel is a mock implementation of the Model interface.
type MockModel struct {
	Table string
	Cols  []string
	Vals  []any
}

func (m MockModel) TableName() string { return m.Table }
func (m MockModel) Columns() []string { return m.Cols }
func (m MockModel) Values() []any     { return m.Vals }
func (m MockModel) Pointers() []any   { return nil }

// MockTxBound is a mock implementation of the TxBound interface.
type MockTxBound struct {
	MockAdapter
	CommitCalled   bool
	RollbackCalled bool
	CommitErr      error
	RollbackErr    error
}

func (m *MockTxBound) Commit() error {
	m.CommitCalled = true
	return m.CommitErr
}

func (m *MockTxBound) Rollback() error {
	m.RollbackCalled = true
	return m.RollbackErr
}

// MockTxAdapter is a mock implementation of the TxAdapter interface.
type MockTxAdapter struct {
	MockAdapter
	Bound *MockTxBound
	BeginTxErr error
}

func (m *MockTxAdapter) BeginTx() (orm.TxBound, error) {
	if m.BeginTxErr != nil {
		return nil, m.BeginTxErr
	}
	if m.Bound == nil {
		m.Bound = &MockTxBound{}
	}
	return m.Bound, nil
}
