package abango

import (
	"encoding/json"
	"os"

	"github.com/go-xorm/xorm"
)

var (
	XEnv *EnvConf
	XDb  *xorm.Engine
)

type RunConf struct {
	RunMode     string
	DevPrefix   string
	ProdPrefix  string
	ConfPostFix string
}

type EnvConf struct {
	AppName      string
	HttpProtocol string
	HttpAddr     string
	HttpPort     string
	SiteName     string

	DbType     string
	DbHost     string
	DbUser     string
	DbPassword string
	DbPort     string
	DbName     string
	DbPrefix   string
	DbTimezone string

	DbStr string
}

func GetEnvConf() error {

	conf := "conf/"
	RunFilename := conf + "run_conf.json"

	var run RunConf

	if file, err := os.Open(RunFilename); err != nil {
		return err
	} else {
		decoder := json.NewDecoder(file)
		err = decoder.Decode(&run)
	}

	filename := ""
	if run.RunMode == "prod" {
		filename = conf + run.ProdPrefix + run.ConfPostFix
	} else if run.RunMode == "dev" {
		filename = conf + run.DevPrefix + run.ConfPostFix
	}

	if file, err := os.Open(filename); err != nil {
		return err
	} else {
		decoder := json.NewDecoder(file)
		err = decoder.Decode(&XEnv)
	}

	if XEnv.DbType == "mysql" {
		XEnv.DbStr = XEnv.DbUser + ":" + XEnv.DbPassword + "@tcp(" + XEnv.DbHost + ":" + XEnv.DbPort + ")/" + XEnv.DbPrefix + XEnv.DbName + "?charset=utf8"
	} else if XEnv.DbType == "mssql" {
		// Add on more DbStr of Db types
	}

	return nil
}
