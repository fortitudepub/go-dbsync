package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

type QueryResult struct {
	Headers          []string
	Rows             [][]string
	Error            string
	ExecutionTime    string
	CostTime         string
	TableName        string
	PrimaryKeysIndex []int
	RowUpdateReady   bool
}

func serveQuery(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	querySql := strings.TrimSpace(req.FormValue("sql"))
	tid := strings.TrimSpace(req.FormValue("tid"))

	dbDataSource, err := selectDb(tid)
	if err != nil {
		http.Error(w, err.Error(), 405)
		return
	}

	tableName, primaryKeys, authAllowed := parseSql(w, querySql, dbDataSource)
	if !authAllowed {
		return
	}

	headers, rows, executionTime, costTime, err := processSqlHistory(querySql, dbDataSource)
	primaryKeysIndex := findPrimaryKeysIndex(tableName, primaryKeys, headers)
	rowUpdateReady := tableName != "" && len(primaryKeys) == len(primaryKeysIndex)

	queryResult := QueryResult{Headers: headers, Rows: rows, Error: gotErrorMessage(err),
		ExecutionTime: executionTime, CostTime: costTime,
		TableName: tableName,
		PrimaryKeysIndex: primaryKeysIndex,
		RowUpdateReady: rowUpdateReady}

	json.NewEncoder(w).Encode(queryResult)
}

func processSqlHistory(querySql, dbDataSource string) ([]string, [][]string, string, string, error) {
	isShowHistory := strings.EqualFold("show history", querySql)
	if isShowHistory {
		return showHistory()
	} else {
		saveHistory(querySql)
		return executeQuery(querySql, dbDataSource)
	}
}

func gotErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
