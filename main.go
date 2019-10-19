package main

import (
	"bitbucket.org/meklis/helpprovider_snmp/logger"
	"flag"
	"fmt"
	"github.com/meklis/telnet-proxy/binder"
	"github.com/meklis/telnet-proxy/config"
	"github.com/meklis/telnet-proxy/poller"
	"github.com/meklis/telnet-proxy/structs"
	"github.com/ztrue/tracerr"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type CommandType string

const (
	INCORRECT_COMMAND CommandType = "INCORRECT_COMMAND"
	CONN              CommandType = "CONN"
	CONN_LIST         CommandType = "CONN_LIST"
)

var (
	Config     config.Configuration
	configPath string
	lg         *logger.Logger
	poll       *poller.Poller
)

func init() {
	flag.StringVar(&configPath, "c", "telnet-proxy.conf.yml", "Configuration file for proxy-auth module")
	flag.Parse()
}
func main() {
	//Read startup configuration
	fmt.Println("Initializing...")
	if err := config.LoadConfig(configPath, &Config); err != nil {
		tracerr.PrintSource(err)
		os.Exit(2)
	}
	//Initialize logger
	_initLogger()

	// Listen for incoming connections.
	err, connType, interfaceAddr, port := config.ParseBind(Config.System.BindAddr)
	if err != nil {
		tracerr.PrintSource(err)
		os.Exit(1)
	}
	l, err := net.Listen(connType, fmt.Sprintf("%v:%v", interfaceAddr, port))
	if err != nil {
		tracerr.PrintSource(err)
		os.Exit(1)
	}

	poll = poller.Init(Config.System.Stream.MaxConn, Config.System.Stream.MaxConnPerHost)

	fmt.Println("Started")
	lg.InfoF("listening bind channel on %v", Config.System.BindAddr)
	for {
		conn, err := l.Accept()
		// Listen for an incoming connection.
		lg.NoticeF("new incomming connection from %v", l.Addr().String())
		if err != nil {
			lg.Errorf("error accepting connection: ", tracerr.Sprint(err))
			continue
		}
		// Handle connections in a new goroutine.
		go handleRequest(conn)
	}
}
func _initLogger() {
	if Config.Logger.Console.Enabled {
		color := 0
		if Config.Logger.Console.EnabledColor {
			color = 1
		}
		lg, _ = logger.New("pooler", color, os.Stdout)
		lg.SetLogLevel(logger.LogLevel(Config.Logger.Console.LogLevel))
		if Config.Logger.Console.LogLevel < 5 {
			lg.SetFormat("#%{id} %{time} > %{level} %{message}")
		} else {
			lg.SetFormat("#%{id} %{time} (%{filename}:%{line}) > %{level} %{message}")
		}
	} else {
		lg, _ = logger.New("no_log", 0, os.DevNull)
	}
}

// Handles incoming requests.
func handleRequest(conn net.Conn) {
	defer func() {
		lg.NoticeF("Client %v disconnected, connection closed", conn.RemoteAddr())
		conn.Close()
	}()

	// Make a buffer to hold incoming data.
	buf := make([]byte, 1024)

	//Hello message
	conn.Write([]byte(`
Telnet proxy v0.1
Supported commands:
	CONNECT <IP Address> <Port>
`))

	//Block of command reading
READ_COMMAND:
	conn.SetDeadline(time.Now().Add(Config.System.DeadLineTimeout))
	conn.Write([]byte("\n>>> "))
	n, err := conn.Read(buf)
	if err != nil {
		lg.InfoF("No commands from client, connection closed")
		return
	}
	err, command, arguments := parseCommand(buf[:n])
	if err != nil {
		lg.Errorf("error parse command: %v", err.Error())
		conn.Write([]byte("\n>>>ERROR_PARSE_COMMAND<<< Message: " + err.Error()))
		goto READ_COMMAND
	}

	switch command {
	case CONN:
		args := arguments.(*structs.Connect)
		if !poll.IsConnectAllowed(args.Ip) {
			conn.Write([]byte(fmt.Sprintf("\n>>>CONNECTION_LIMIT<<< Message: connect to %v is denied by limits\n", args.Ip)))
			goto READ_COMMAND
		}
		conn.Write([]byte(fmt.Sprintf("\n>>>CONNECTING<<< Message: connect to %v:%v\n", args.Ip, args.Port)))
		err := connect(conn, args.Ip, args.Port)
		conn.Write([]byte(fmt.Sprintf(`
>>>CONNECTION_CLOSED<<< Message: %v
`, err)))
		return
	case CONN_LIST:
		args := arguments.(*structs.Connect)
		if !poll.IsConnectAllowed(args.Ip) {
			conn.Write([]byte(fmt.Sprintf("\n>>>CONNECTION_LIMIT<<< Message: connect to %v is denied by limits\n", args.Ip)))
			goto READ_COMMAND
		}
		conn.Write([]byte(fmt.Sprintf("\n>>>CONNECTING<<< Message: connect to %v:%v\n", args.Ip, args.Port)))
		err := connect(conn, args.Ip, args.Port)
		conn.Write([]byte(fmt.Sprintf(`
>>>CONNECTION_CLOSED<<< Message: %v
`, err)))
		return
	default:
		if getPreparedLine(string(buf[:n])) == "" {
			goto READ_COMMAND
		}
		lg.Errorf("incorrect command (%v) ", getPreparedLine(string(buf[:n])))
		conn.Write([]byte(fmt.Sprintf("\n>>>INCORRECT_OR_NOT_SUPPORTED_COMMAND<<<")))
	}
	goto READ_COMMAND
}

func parseCommand(b []byte) (err error, command CommandType, argument interface{}) {
	formated := getPreparedLine(string(b))
	exploded := strings.Split(formated, " ")
	for num, expl := range exploded {
		exploded[num] = strings.Trim(expl, "\n")
	}

	if len(exploded) < 1 {
		return fmt.Errorf("incorrect command argument"), "", nil
	}

	switch CommandType(exploded[0]) {
	case CONN:
		if len(exploded) < 3 {
			return fmt.Errorf("not all arguments received for connect"), INCORRECT_COMMAND, nil
		}
		Ip := exploded[1]
		Port, err := strconv.Atoi(exploded[2])
		if !validIP4(Ip) {
			return fmt.Errorf("incorrect IP address received in argument"), INCORRECT_COMMAND, nil
		}
		if err != nil {
			return fmt.Errorf("error parse %v as port number : %v", Port, err), INCORRECT_COMMAND, nil
		}
		return nil, CONN, &structs.Connect{
			Port: Port,
			Ip:   Ip,
		}
	case CONN_LIST:
		return nil, CONN_LIST, nil
	}
	return nil, INCORRECT_COMMAND, nil
}
func getPreparedLine(line string) string {
	line = strings.ReplaceAll(line, "\r\n", "")
	line = strings.ReplaceAll(line, "\n", "")
	line = strings.ReplaceAll(line, "\x00", "")
	return line
}

func validIP4(ipAddress string) bool {
	ipAddress = strings.Trim(ipAddress, " ")

	re, _ := regexp.Compile(`^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])$`)
	if re.MatchString(ipAddress) {
		return true
	}
	return false
}

func connect(conn net.Conn, ip string, port int) error {
	//Initialize global variables
	lg.NoticeF("channel %v try open connect to %v", conn.RemoteAddr().String(), fmt.Sprintf("%v:%v", ip, port))
	//one := []byte{}
	//Open connection
	d := net.Dialer{
		Timeout: Config.System.Stream.ConnTimeout,
	}
	telnet, err := d.Dial("tcp", fmt.Sprintf("%v:%v", ip, port))
	if err != nil {
		return tracerr.Wrap(err)
	}
	lg.NoticeF("success open connect to %v from channel %v", fmt.Sprintf("%v:%v", ip, port), conn.RemoteAddr().String())
	defer func() {
		lg.NoticeF("Device %v disconnected, connection closed", telnet.RemoteAddr())
		telnet.Close()
	}()

	poll.AddBind(poller.Bind{
		Device: telnet.RemoteAddr().String(),
		Client: conn.RemoteAddr().String(),
	})

	bind := binder.InitBinder(
		binder.BinderConfig{
			ClientTimeout: Config.System.DeadLineTimeout,
			DeviceTimeout: Config.System.Stream.DeadLineTimeout,
			Logger:        lg,
		})

	err, message := bind.BindChannel(conn, telnet).Wait()
	bind.CloseBinder()
	poll.DeleteBind(poller.Bind{
		Device: telnet.RemoteAddr().String(),
		Client: conn.RemoteAddr().String(),
	})
	if err != nil {
		return tracerr.Wrap(err)
	} else {
		return tracerr.Errorf(message)
	}
}
