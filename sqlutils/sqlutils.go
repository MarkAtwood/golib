/*
   Copyright 2014 Outbrain Inc.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package sqlutils

import (
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/outbrain/log"
	"strconv"
	"strings"
)

// RowMap represents one row in a result set. Its objective is to allow
// for easy, typed getters by column name.
type RowMap map[string]sql.NullString

func (this *RowMap) GetString(key string) string {
	return (*this)[key].String
}

func (this *RowMap) GetInt64(key string) int64 {
	res, _ := strconv.ParseInt(this.GetString(key), 10, 0)
	return res
}

func (this *RowMap) GetNullInt64(key string) sql.NullInt64 {
	i, err := strconv.ParseInt(this.GetString(key), 10, 0)
	if err == nil {
		return sql.NullInt64{Int64: i, Valid: true}
	} else {
		return sql.NullInt64{Valid: false}
	}
}

func (this *RowMap) GetInt(key string) int {
	res, _ := strconv.Atoi(this.GetString(key))
	return res
}

func (this *RowMap) GetIntD(key string, def int) int {
	res, err := strconv.Atoi(this.GetString(key))
	if err != nil {
		return def
	}
	return res
}

func (this *RowMap) GetUint(key string) uint {
	res, _ := strconv.Atoi(this.GetString(key))
	return uint(res)
}

func (this *RowMap) GetBool(key string) bool {
	return this.GetInt(key) != 0
}

// knownDBs is a DB cache by uri
var knownDBs map[string]*sql.DB = make(map[string]*sql.DB)

// GetDB returns a DB instance based on uri.
// bool result indicates whether the DB was returned from cache; err
func GetDB(mysql_uri string) (*sql.DB, bool, error) {

	var exists bool
	if _, exists = knownDBs[mysql_uri]; !exists {
		if db, err := sql.Open("mysql", mysql_uri); err == nil {
			knownDBs[mysql_uri] = db
		} else {
			return db, exists, err
		}
	}
	return knownDBs[mysql_uri], exists, nil
}

// RowToArray is a convenience function, typically not called directly, which maps a
// single read database row into a NullString
func RowToArray(rows *sql.Rows, columns []string) []sql.NullString {
	buff := make([]interface{}, len(columns))
	data := make([]sql.NullString, len(columns))
	for i, _ := range buff {
		buff[i] = &data[i]
	}
	rows.Scan(buff...)
	return data
}

// ScanRowsToArrays is a convenience function, typically not called directly, which maps rows
// already read from the databse into arrays of NullString
func ScanRowsToArrays(rows *sql.Rows, on_row func([]sql.NullString) error) error {
	columns, _ := rows.Columns()
	for rows.Next() {
		arr := RowToArray(rows, columns)

		err := on_row(arr)
		if err != nil {
			return err
		}
	}
	return nil
}

// ScanRowsToMaps is a convenience function, typically not called directly, which maps rows
// already read from the databse into RowMap entries.
func ScanRowsToMaps(rows *sql.Rows, on_row func(RowMap) error) error {
	columns, _ := rows.Columns()
	err := ScanRowsToArrays(rows, func(arr []sql.NullString) error {
		m := make(map[string]sql.NullString)
		for k, data_col := range arr {
			m[columns[k]] = data_col
		}
		err := on_row(m)
		if err != nil {
			return err
		}
		return nil
	})
	return err
}

// QueryRowsMap is a convenience function allowing querying a result set while poviding a callback
// function activated per read row.
func QueryRowsMap(db *sql.DB, query string, on_row func(RowMap) error) error {
	var err error
	defer func() {
		if derr := recover(); derr != nil {
			err = errors.New(fmt.Sprintf("QueryRowsMap unexpected error: %+v", derr))
		}
	}()

	rows, err := db.Query(query)
	defer rows.Close()
	if err != nil && err != sql.ErrNoRows {
		return log.Errore(err)
	}
	err = ScanRowsToMaps(rows, on_row)
	return err
}

// ExecQuery executes given query using given args on given DB. It will safele prepare, execute and close
// the statement.
func execInternal(silent bool, db *sql.DB, query string, args ...interface{}) (sql.Result, error) {
	var err error
	defer func() {
		if derr := recover(); derr != nil {
			err = errors.New(fmt.Sprintf("QueryRowsMap unexpected error: %+v", derr))
		}
	}()

	stmt, err := db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var res sql.Result
	res, err = stmt.Exec(args...)
	if err != nil && !silent {
		log.Errore(err)
	}
	return res, err
}

// Exec executes given query using given args on given DB. It will safele prepare, execute and close
// the statement.
func Exec(db *sql.DB, query string, args ...interface{}) (sql.Result, error) {
	return execInternal(false, db, query, args...)
}

// ExecSilently acts like Exec but does not report any error
func ExecSilently(db *sql.DB, query string, args ...interface{}) (sql.Result, error) {
	return execInternal(true, db, query, args...)
}

func InClauseStringValues(terms []string) string {
	quoted := []string{}
	for _, s := range terms {
		quoted = append(quoted, fmt.Sprintf("'%s'", strings.Replace(s, ",", "''", -1)))
	}
	return strings.Join(quoted, ", ")
}
