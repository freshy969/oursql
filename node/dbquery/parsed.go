package dbquery

import (
	"encoding/base64"
	"regexp"

	"github.com/gelembjuk/oursql/lib"
	"github.com/gelembjuk/oursql/node/database"
	"github.com/gelembjuk/oursql/node/dbquery/sqlparser"
)

type QueryParsed struct {
	SQL              string
	PubKey           []byte
	Signature        []byte
	TransactionBytes []byte
	KeyCol           string
	KeyVal           string
	RowBeforeQuery   map[string]string
	Structure        sqlparser.SQLQueryParserInterface
}

func (qp QueryParsed) ReferenceID() string {
	return qp.Structure.GetTable() + ":" + qp.KeyVal
}
func (qp QueryParsed) GetKeyValue() string {
	return qp.KeyVal
}

// Info about a parsed query. Check if is select
func (qp QueryParsed) IsSelect() bool {
	return qp.Structure.GetKind() == lib.QueryKindSelect
}

// Info about a parsed query. Check if is update (insert, update, delete, create table, drop table)
func (qp QueryParsed) IsUpdate() bool {
	return qp.Structure.GetKind() == lib.QueryKindCreate ||
		qp.Structure.GetKind() == lib.QueryKindDrop ||
		qp.Structure.GetKind() == lib.QueryKindDelete ||
		qp.Structure.GetKind() == lib.QueryKindInsert ||
		qp.Structure.GetKind() == lib.QueryKindUpdate
}

// prepares rollback query
func (qp QueryParsed) buildRollbackSQL() (string, error) {
	if qp.Structure.GetKind() == lib.QueryKindCreate {
		return "DROP TABLE " + qp.Structure.GetTable(), nil
	}
	if qp.Structure.GetKind() == lib.QueryKindDrop {
		// no rollback for this operation . this must be processed somehow differently
		return "", nil
	}
	if qp.Structure.GetKind() == lib.QueryKindInsert {

		return "DELETE FROM " + qp.Structure.GetTable() + " WHERE " + qp.KeyCol + "='" + database.Quote(qp.KeyVal) + "'", nil
	}
	return "", nil
}

// Parse comments
func (qp QueryParsed) parseInfoFromComments() (PubKey []byte, Signature []byte, TransactionBytes []byte, err error) {
	PubKey = []byte{}
	Signature = []byte{}
	TransactionBytes = []byte{}

	comments := qp.Structure.GetComments()

	if len(comments) == 0 {
		return
	}

	comment := comments[0]

	var r *regexp.Regexp

	r, err = regexp.Compile("SIGN:([^;]+);")

	if err != nil {
		return
	}

	s := r.FindAllString(comment, -1)

	if len(s) == 2 {
		Signature, err = base64.StdEncoding.DecodeString(s[1])

		if err != nil {
			return
		}
	}

	r, err = regexp.Compile("DATA:([^;]+);")

	if err != nil {
		return
	}

	s = r.FindAllString(comment, -1)

	if len(s) == 2 {
		TransactionBytes, err = base64.StdEncoding.DecodeString(s[1])

		if err != nil {
			return
		}
	}

	r, err = regexp.Compile("PUBKEY:([^;]+);")

	if err != nil {
		return
	}

	s = r.FindAllString(comment, -1)

	if len(s) == 2 {
		PubKey, err = base64.StdEncoding.DecodeString(s[1])

		if err != nil {
			return
		}
	}

	return
}