package orm

import "github.com/tinywasm/fmt"

// RenameProvider is implemented by generated models when db:"old_name=X" tags are present.
type RenameProvider interface {
	OldNames() map[string]string
}

// TableIntrospector is optionally implemented by the injected Executor to retrieve column names.
type TableIntrospector interface {
	TableColumns(table string) ([]string, error)
}

// Sync reconciles the database to match the given models.
// Emits CreateTable, AddColumn, RenameColumn, and DropColumn Actions;
// the engine adapter compiles and executes the dialect SQL.
func (db *DB) Sync(models ...fmt.Model) error {
	if len(models) == 0 {
		return nil
	}

	// Wrap in transaction if available
	if _, ok := db.exec.(TxExecutor); ok {
		return db.Tx(func(db *DB) error {
			return db.syncAll(models...)
		})
	}

	return db.syncAll(models...)
}

func (db *DB) syncAll(models ...fmt.Model) error {
	for _, m := range models {
		if err := db.syncModel(m); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) syncModel(m fmt.Model) error {
	tableName := m.ModelName()
	schema := m.Schema()

	// 1. Emit ActionCreateTable (idempotent)
	if err := db.CreateTable(m); err != nil {
		return wrapSyncErr(err)
	}

	// 2. Cast Executor to TableIntrospector
	introspector, ok := db.exec.(TableIntrospector)

	if !ok {
		// Fallback to purely additive loop
		for _, field := range schema {
			qAdd := Query{
				Action: ActionAddColumn,
				Table:  tableName,
				Column: &field,
			}
			// Catch duplicates via log-and-continue (logged by the engine adapter)
			_ = db.execQuery(qAdd, m)
		}
		return nil
	}

	// 3. Retrieve existing columns
	existingCols, err := introspector.TableColumns(tableName)
	if err != nil {
		return wrapSyncErr(err)
	}

	existingMap := make(map[string]bool)
	for _, col := range existingCols {
		existingMap[col] = true
	}

	// 4. Check if model implements RenameProvider
	var oldNames map[string]string
	if rp, ok := m.(RenameProvider); ok {
		oldNames = rp.OldNames()
	}

	// 5. Reconcile Adds & Renames
	for _, field := range schema {
		if existingMap[field.Name] {
			continue
		}

		oldName, hasOld := oldNames[field.Name]
		if hasOld && existingMap[oldName] {
			qRename := Query{
				Action:  ActionRenameColumn,
				Table:   tableName,
				Column:  &field,
				OldName: oldName,
			}
			_ = db.execQuery(qRename, m)
		} else {
			qAdd := Query{
				Action: ActionAddColumn,
				Table:  tableName,
				Column: &field,
			}
			_ = db.execQuery(qAdd, m)
		}
	}

	// 6. Reconcile Safe Drops
	schemaMap := make(map[string]bool)
	for _, field := range schema {
		schemaMap[field.Name] = true
	}
	// Also account for columns that were just renamed FROM something
	renamedFrom := make(map[string]bool)
	for _, old := range oldNames {
		renamedFrom[old] = true
	}

	for _, col := range existingCols {
		if schemaMap[col] || renamedFrom[col] {
			continue
		}

		// Run safe check: SELECT 1 FROM <table> WHERE <col> IS NOT NULL LIMIT 1
		qCheck := Query{
			Action:  ActionReadAll,
			Table:   tableName,
			Columns: []string{"1"},
			Conditions: []Condition{
				IsNotNull(col),
			},
			Limit: 1,
		}

		plan, err := db.compiler.Compile(qCheck, m)
		if err != nil {
			continue
		}

		rows, err := db.exec.Query(plan.Query, plan.Args...)
		if err != nil {
			continue
		}

		hasData := rows.Next()
		rows.Close()

		if !hasData {
			qDrop := Query{
				Action:  ActionDropColumn,
				Table:   tableName,
				Columns: []string{col},
			}
			_ = db.execQuery(qDrop, m)
		}
	}

	return nil
}

func (db *DB) execQuery(q Query, m fmt.Model) error {
	plan, err := db.compiler.Compile(q, m)
	if err != nil {
		return err
	}
	return db.exec.Exec(plan.Query, plan.Args...)
}
