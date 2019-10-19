package config

import (
	"fmt"
	"github.com/ztrue/tracerr"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"regexp"
	"strconv"
	"time"
)

type Configuration struct {
	System struct {
		BindAddr         string        `yaml:"bind_addr"`
		DeadLineTimeout  time.Duration `yaml:"deadline_timeout"`
		KeepAliveTimeout time.Duration `yaml:"keep_alive_timeout"`
		Stream           struct {
			DeadLineTimeout time.Duration `yaml:"deadline_timeout"`
			MaxConnPerHost  int           `yaml:"max_connection_per_host"`
			MaxConn         int           `yaml:"max_connections"`
			ConnTimeout     time.Duration `yaml:"connection_timeout"`
		} `yaml:"stream"`
	} `yaml:"system"`
	Logger struct {
		Console struct {
			Enabled      bool `yaml:"enabled"`
			EnabledColor bool `yaml:"enable_color"`
			LogLevel     int  `yaml:"log_level"`
		} `yaml:"console"`
	} `yaml:"logger"`
	Http struct {
		Enabled  bool   `yaml:"enabled"`
		BindAddr string `yaml:"bind_addr"`
		Path     string `yaml:"path"`
	} `yaml:"http"`
	Prometheus struct {
		Enabled  bool   `yaml:"enabled"`
		BindAddr string `yaml:"bind_addr"`
		Path     string `yaml:"path"`
	} `yaml:"prometheus"`
}

func LoadConfig(pathConfig string, configuration *Configuration) error {
	bytes, err := ioutil.ReadFile(pathConfig)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(bytes, configuration)
	if err != nil {
		return err
	}
	return nil
}

func ParseBind(bindStr string) (err error, connType, interfaceAddr string, port int) {
	regex, err := regexp.Compile(`^(.*?):\/\/(.*?):(.*)$`)
	if err != nil {
		return tracerr.Wrap(err), "", "", 0
	}
	res := regex.FindAllStringSubmatch(bindStr, -1)
	if len(res) > 0 {
		if len(res[0]) < 4 {
			return tracerr.Wrap(fmt.Errorf("bind address is incorrect, check configuration")), "", "", 0
		}
		port, err := strconv.Atoi(res[0][3])
		if err != nil {
			return tracerr.Wrap(err), "", "", 0
		}
		return nil, res[0][1], res[0][2], port
	} else {
		return tracerr.Wrap(fmt.Errorf("bind address is incorrect, check configuration")), "", "", 0
	}
}
