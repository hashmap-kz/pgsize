package ui

import (
	"github.com/hashmap-kz/pgsize/internal/pg"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jackc/pgx/v5/pgxpool"
)

type viewKind int

const (
	viewClusters viewKind = iota
	viewDatabases
	viewSchemas
	viewTables
	viewRelations
	viewTopTables
)

const (
	keyBackspace = "backspace"
	colSize      = "SIZE"
	nilByte      = "\x00"
)

type sortMode int

const (
	sortSize sortMode = iota
	sortName
)

// Cluster holds the connection details for one PostgreSQL cluster.
type Cluster struct {
	Name string
	DSN  string
	Pool *pgxpool.Pool
}

type clusterState struct {
	Cluster
	dbCache     []pg.Database
	schCache    map[string][]pg.Schema
	tblCache    map[string][]pg.Table
	relCache    map[string][]pg.Relation
	topTblCache map[string][]pg.Table // keyed by db name
}

type model struct {
	clusters   []clusterState
	curCluster int

	view    viewKind
	dbs     []pg.Database
	schs    []pg.Schema
	tbls    []pg.Table
	rels    []pg.Relation
	topTbls []pg.Table
	stack   []frame
	cursor  int
	sort    sortMode

	curDB     string
	curSchema string
	curTable  string

	loading bool
	loadID  uint64

	width  int
	height int
	err    error
}

var _ tea.Model = &model{}

type frame struct {
	view   viewKind
	cursor int
	curDB  string
	curSch string
	curTbl string
}

// messages

type loadedDatabases struct {
	loadID     uint64
	clusterIdx int
	items      []pg.Database
	err        error
}
type loadedSchemas struct {
	loadID     uint64
	clusterIdx int
	db         string
	items      []pg.Schema
	err        error
}
type loadedTables struct {
	loadID     uint64
	clusterIdx int
	db         string
	schema     string
	items      []pg.Table
	err        error
}
type loadedRelations struct {
	loadID     uint64
	clusterIdx int
	db         string
	schema     string
	table      string
	items      []pg.Relation
	err        error
}
type loadedTopTables struct {
	loadID     uint64
	clusterIdx int
	db         string
	items      []pg.Table
	err        error
}

func InitialModel(clusters []Cluster) tea.Model {
	cs := make([]clusterState, len(clusters))
	for i, c := range clusters {
		cs[i] = clusterState{
			Cluster:     c,
			schCache:    make(map[string][]pg.Schema),
			tblCache:    make(map[string][]pg.Table),
			relCache:    make(map[string][]pg.Relation),
			topTblCache: make(map[string][]pg.Table),
		}
	}
	view := viewClusters
	if len(cs) == 1 {
		view = viewDatabases
	}
	return &model{clusters: cs, view: view, sort: sortSize}
}

func (m *model) Init() tea.Cmd                           { return m.initLoad() }
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return m.handleMsg(msg) }
func (m *model) View() string                            { return m.renderView() }
