package main

import (
	"database/sql"
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"time"

	gs "github.com/HeroesAwaken/GoAwaken/GameSpy"
	log "github.com/HeroesAwaken/GoAwaken/Log"
	"github.com/HeroesAwaken/GoAwaken/core"
	"github.com/go-redis/redis"
)

type GameServer struct {
	ip                 string
	port               string
	intIP              string
	intPort            string
	name               string
	level              string
	activePlayers      int
	maxPlayers         int
	queueLength        int
	joiningPlayers     int
	gameMode           string
	elo                float64
	numObservers       int
	maxObservers       int
	sguid              string
	hash               string
	password           string
	ugid               string
	sType              string
	join               string
	version            string
	dataCenter         string
	serverMap          string
	armyBalance        string
	armyDistribution   string
	availSlotsNational bool
	availSlotsRoyal    bool
	avgAllyRank        float64
	avgAxisRank        float64
	serverState        string
	communityName      string
}

type TheaterManager struct {
	name             string
	socket           *gs.Socket
	socketUDP        *gs.SocketUDP
	db               *sql.DB
	redis            *redis.Client
	eventsChannel    chan gs.SocketEvent
	eventsChannelUDP chan gs.SocketUDPEvent
	batchTicker      *time.Ticker
	stopTicker       chan bool
	gameServerGlobal *core.RedisState
}

var wantsToJoin bool = false
var localPort string = ""

// New creates and starts a new ClientManager
func (tM *TheaterManager) New(name string, port string, db *sql.DB, redis *redis.Client) {
	var err error

	tM.socket = new(gs.Socket)
	tM.socketUDP = new(gs.SocketUDP)
	tM.db = db
	tM.redis = redis
	tM.name = name
	tM.eventsChannel, err = tM.socket.New(tM.name, port, true)
	if err != nil {
		log.Errorln(err)
	}
	tM.eventsChannelUDP, err = tM.socketUDP.New(tM.name, port, true)
	if err != nil {
		log.Errorln(err)
	}
	tM.stopTicker = make(chan bool, 1)

	tM.gameServerGlobal = new(core.RedisState)
	tM.gameServerGlobal.New(tM.redis, "gameServer-config")
	tM.gameServerGlobal.Set("Lobbies", "0")

	go tM.run()
}

func (tM *TheaterManager) run() {
	for {
		select {
		case event := <-tM.eventsChannelUDP:
			switch {
			case event.Name == "command.ECHO":
				go tM.ECHO(event)
			case event.Name == "command":
				tM.LogCommandUDP(event.Data.(*gs.CommandFESL))
				log.Debugf("UDP Got event %s: %v", event.Name, event.Data.(*gs.CommandFESL))
			default:
				log.Debugf("UDP Got event %s: %v", event.Name, event.Data)
			}
		case event := <-tM.eventsChannel:
			switch {
			case event.Name == "newClient":
				go tM.newClient(event.Data.(gs.EventNewClient))
			case event.Name == "client.command.CONN":
				go tM.CONN(event.Data.(gs.EventClientFESLCommand))
			case event.Name == "client.command.USER":
				go tM.USER(event.Data.(gs.EventClientFESLCommand))
			case event.Name == "client.command.LLST":
				go tM.LLST(event.Data.(gs.EventClientFESLCommand))
			case event.Name == "client.command.GDAT":
				go tM.GDAT(event.Data.(gs.EventClientFESLCommand))
			case event.Name == "client.command.EGAM":
				go tM.EGAM(event.Data.(gs.EventClientFESLCommand))
			case event.Name == "client.command.ECNL":
				go tM.ECNL(event.Data.(gs.EventClientFESLCommand))
			case event.Name == "client.command.CGAM":
				go tM.CGAM(event.Data.(gs.EventClientFESLCommand))
			case event.Name == "client.command.UBRA":
				go tM.UBRA(event.Data.(gs.EventClientFESLCommand))
			case event.Name == "client.command.UGAM":
				go tM.UGAM(event.Data.(gs.EventClientFESLCommand))
			case event.Name == "client.command.EGRS":
				go tM.EGRS(event.Data.(gs.EventClientFESLCommand))
			case event.Name == "client.command":
				tM.LogCommand(event.Data.(gs.EventClientFESLCommand))
				log.Debugf("Got event %s: %v", event.Name, event.Data.(gs.EventClientFESLCommand).Command)
			default:
				log.Debugf("Got event %s: %v", event.Name, event.Data)
			}
		}
	}
}

func (tM *TheaterManager) ECHO(event gs.SocketUDPEvent) {
	command := event.Data.(*gs.CommandFESL)

	answerPacket := make(map[string]string)
	answerPacket["TID"] = command.Message["TID"]
	answerPacket["TXN"] = command.Message["TXN"]
	answerPacket["IP"] = event.Addr.IP.String()
	answerPacket["PORT"] = strconv.Itoa(event.Addr.Port)
	answerPacket["ERR"] = "0"
	answerPacket["TYPE"] = "1"
	err := tM.socketUDP.WriteFESL("ECHO", answerPacket, 0x0, event.Addr)
	if err != nil {
		log.Errorln(err)
	}
	tM.logAnswer("ECHO", answerPacket, 0x0)
}

func (tM *TheaterManager) ECNL(event gs.EventClientFESLCommand) {
	if !event.Client.IsActive {
		log.Noteln("Client left")
		return
	}

	answerPacket := make(map[string]string)
	answerPacket["TID"] = event.Command.Message["TID"]
	answerPacket["GID"] = event.Command.Message["GID"]
	answerPacket["LID"] = event.Command.Message["LID"]
	event.Client.WriteFESL("ECNL", answerPacket, 0x0)
	tM.logAnswer("ECNL", answerPacket, 0x0)
}

func (tM *TheaterManager) EGAM(event gs.EventClientFESLCommand) {
	if !event.Client.IsActive {
		log.Noteln("Client left")
		return
	}

	answerPacket := make(map[string]string)
	answerPacket["TID"] = event.Command.Message["TID"]
	answerPacket["GID"] = "1"
	answerPacket["LID"] = "1"

	localPort = event.Command.Message["R-INT-PORT"]
	wantsToJoin = true

	event.Client.WriteFESL("EGAM", answerPacket, 0x0)
	tM.logAnswer("EGAM", answerPacket, 0x0)

	ap := make(map[string]string)
	ap["PL"] = "pc"
	ap["TICKET"] = "1234"
	ap["PID"] = "100"
	ap["I"] = "192.168.69.7"
	ap["P"] = "18567"
	ap["HUID"] = "1817224672"
	ap["EKEY"] = "8564"
	ap["INT-IP"] = "192.168.69.7"
	ap["INT-PORT"] = "18567"
	ap["UGID"] = "test123"
	ap["LID"] = "1"
	ap["GID"] = "1"
	event.Client.WriteFESL("EGEG", ap, 0x0)

	tM.logAnswer("EGEG", ap, 0x0)
	log.Noteln(ap)
	//event.Client.WriteFESL("EGAM", answerPacket, 0x0)
	//tM.logAnswer("EGAM", answerPacket, 0x0)
}

func (tM *TheaterManager) CGAM(event gs.EventClientFESLCommand) {
	if !event.Client.IsActive {
		log.Noteln("Client left")
		return
	}

	addr, ok := event.Client.IpAddr.(*net.TCPAddr)

	if !ok {
		log.Errorln("Failed turning IpAddr to net.TCPAddr")
		return
	}

	currentLobbyId := tM.gameServerGlobal.Get("Lobbies")
	gameLid, _ := strconv.Atoi(currentLobbyId)
	gameLid++

	gameServer := new(core.RedisState)
	gameServer.New(tM.redis, "gameServer-"+strconv.Itoa(gameLid))

	for index, value := range event.Command.Message {
		// Strip quotes
		if len(value) > 0 && value[0] == '"' {
			value = value[1:]
		}
		if len(value) > 0 && value[len(value)-1] == '"' {
			value = value[:len(value)-1]
		}
		gameServer.Set(index, value)
	}

	gameServer.Set("LID", strconv.Itoa(gameLid))
	gameServer.Set("IP", addr.IP.String())
	gameServer.Set("ACTIVE-PLAYERS", "0")
	gameServer.Set("QUEUE-LENGTH", "0")

	answerPacket := make(map[string]string)
	answerPacket["TID"] = event.Command.Message["TID"]
	answerPacket["MAX-PLAYERS"] = "16"
	answerPacket["EKEY"] = "8564"
	answerPacket["UGID"] = "test123"
	answerPacket["JOIN"] = event.Command.Message["JOIN"]
	answerPacket["LID"] = "1"
	answerPacket["SECRET"] = "test123" //
	answerPacket["J"] = "0"
	answerPacket["GID"] = "1"
	event.Client.WriteFESL("CGAM", answerPacket, 0x0)
	tM.logAnswer("CGAM", answerPacket, 0x0)

	tM.gameServerGlobal.Set("Lobbies", strconv.Itoa(gameLid))
}

func (tM *TheaterManager) GDAT(event gs.EventClientFESLCommand) {
	if !event.Client.IsActive {
		log.Noteln("Client left")
		return
	}

	gameServer := new(core.RedisState)
	gameServer.New(tM.redis, "gameServer-"+event.Command.Message["LID"])

	answerPacket := make(map[string]string)

	answerPacket["LID"] = "1"
	answerPacket["GID"] = "1"
	answerPacket["TYPE"] = "G"
	answerPacket["HN"] = "mDaWg"
	answerPacket["HU"] = "1817224672"
	answerPacket["N"] = "Test"
	answerPacket["I"] = "192.168.69.7"
	answerPacket["P"] = "18567"
	answerPacket["J"] = "0"
	answerPacket["JP"] = "0"
	answerPacket["QP"] = "0"
	answerPacket["AP"] = "0"
	answerPacket["MP"] = "16"
	answerPacket["F"] = ""
	answerPacket["NF"] = ""
	answerPacket["PL"] = "PC"
	answerPacket["PW"] = "0"
	answerPacket["B-U-EA"] = "1"
	answerPacket["B-U-Softcore"] = "0"
	answerPacket["B-U-Hardcore"] = "0"
	answerPacket["B-U-HasPassword"] = "0"
	answerPacket["B-U-Punkbuster"] = "0"
	answerPacket["B-version"] = "1.02.1067.0"
	answerPacket["V"] = "0"
	answerPacket["B-U-level"] = "village"
	answerPacket["B-U-gamemode"] = "gpm_tdm"
	answerPacket["B-U-sguid"] = ""
	answerPacket["B-U-Time="] = "10"
	answerPacket["B-U-hash"] = ""
	answerPacket["B-U-region"] = ""
	answerPacket["B-U-public"] = "1"
	answerPacket["B-U-elo"] = "0"
	answerPacket["B-numObservers"] = "0"
	answerPacket["B-maxObservers"] = "0"
	answerPacket["B-U-Provider"] = ""
	answerPacket["B-U-gameMod"] = "bfheroes"
	answerPacket["B-U-QueueLength"] = "0"


	event.Client.WriteFESL("GDAT", answerPacket, 0x0)
	tM.logAnswer("GDAT", answerPacket, 0x0)

	/*
		answerPacket["B-maxObservers"] = "0"
		answerPacket["B-numObservers"] = "0"
		answerPacket["B-U-alwaysQueue"] = "1"
		answerPacket["B-U-army_balance"] = "Balanced"
		answerPacket["B-U-army_distribution"] = "0,0,0,0,0,0,0,0,0,0,0"
		answerPacket["B-U-avail_slots_national"] = "yes"
		answerPacket["B-U-avail_slots_royal"] = "yes"
		answerPacket["B-U-avail_vips_national"] = "4"
		answerPacket["B-U-avail_vips_royal"] = "4"
		answerPacket["B-U-avg_ally_rank"] = "1000"
		answerPacket["B-U-avg_axis_rank"] = "1000"
		answerPacket["B-U-community_name"] = "Heroes SV"
		answerPacket["B-U-data_center"] = "iad"
		answerPacket["B-U-easyzone"] = "no"
		answerPacket["B-U-elo_rank"] = "1000"
		answerPacket["B-U-lvl_avg"] = "0.000000"
		answerPacket["B-U-lvl_sdv"] = "0.000000"
		answerPacket["B-U-map"] = "village"
		answerPacket["B-U-map_name"] = "Village"
		answerPacket["B-U-percent_full"] = "0"
		answerPacket["B-U-punkb"] = "0"
		answerPacket["B-U-ranked"] = "yes"
		answerPacket["B-U-server_ip"] = "127.0.0.1"
		answerPacket["B-U-server_port"] = "18567"
		answerPacket["B-U-server_state"] = "empty"
		answerPacket["B-U-servertype"] = "public"
		answerPacket["B-version"] = "1.89.239937.0"

		answerPacket["GID"] = "1"
		answerPacket["I"] = "127.0.0.1"
		answerPacket["J"] = "O"
		answerPacket["LID"] = "1"
		answerPacket["MP"] = "16"
		answerPacket["N"] = "[iad]A Battlefield Heroes Server(127.0.0.1:18567)"
		answerPacket["P"] = "18567"
		answerPacket["PL"] = "PC"
		answerPacket["TID"] = event.Command.Message["TID"]
		answerPacket["TYPE"] = "G"
	*/
	/*
			answerPacket["TID"] = event.Command.Message["TID"]
			answerPacket["LID"] = event.Command.Message["LID"]
			answerPacket["GID"] = event.Command.Message["GID"]

			answerPacket["HU"] = "bfwest-pc"
			answerPacket["HN"] = "1"

			answerPacket["I"] = "127.0.0.1"
			answerPacket["P"] = gameServer.Get("PORT")
			answerPacket["N"] = gameServer.Get("NAME")
			answerPacket["AP"] = gameServer.Get("ACTIVE-PLAYERS")
			answerPacket["MP"] = gameServer.Get("MAX-PLAYERS")
			answerPacket["QP"] = gameServer.Get("QUEUE-LENGTH")
			answerPacket["JP"] = "0"
			answerPacket["PL"] = "PC"

			answerPacket["PW"] = "0"
			answerPacket["TYPE"] = gameServer.Get("TYPE")
			answerPacket["J"] = gameServer.Get("JOIN")

			for _, key := range gameServer.HKeys() {
				if strings.Index(key, "B-") != -1 {
					answerPacket[key] = gameServer.Get(key)
				}
			}

			answerPacket["B-version"] = "1.89.239937.0"
			answerPacket["V"] = "1.89.239937.0"

		answerPacket["TID"] = event.Command.Message["TID"]
		event.Client.WriteFESL("GDAT", answerPacket, 0x0)
		tM.logAnswer("GDAT", answerPacket, 0x0)
	*/

	answerPacket = make(map[string]string)
	answerPacket["TID"] = event.Command.Message["TID"]
	answerPacket["LID"] = "1"
	answerPacket["GID"] = "1"
	answerPacket["D-AutoBalance"] = "1"
	answerPacket["D-Crosshair"] = "1"
	answerPacket["D-FriendlyFire"] = "1"
	answerPacket["D-KillCam"] = "1"
	answerPacket["D-Minimap"] = "1"
	answerPacket["D-MinimapSpotting"] = "1"
	answerPacket["D-ServerDescriptionCount"] = "0"

	answerPacket["D-ThirdPersonVehicleCameras"] = "0"
	answerPacket["D-ThreeDSpotting"] = "0"
	answerPacket["UGID"] = "test123"

	event.Client.WriteFESL("GDET", answerPacket, 0x0)
	tM.logAnswer("GDET", answerPacket, 0x0)

}

func (tM *TheaterManager) LogCommandUDP(event *gs.CommandFESL) {
	b, err := json.MarshalIndent(event.Message, "", "	")
	if err != nil {
		panic(err)
	}

	commandType := "request"

	os.MkdirAll("./commands/"+event.Query+"."+event.Message["TXN"]+"", 0777)
	err = ioutil.WriteFile("./commands/"+event.Query+"."+event.Message["TXN"]+"/"+commandType, b, 0644)
	if err != nil {
		panic(err)
	}
}

func (tM *TheaterManager) LogCommand(event gs.EventClientFESLCommand) {
	b, err := json.MarshalIndent(event.Command.Message, "", "	")
	if err != nil {
		panic(err)
	}

	commandType := "request"

	os.MkdirAll("./commands/"+event.Command.Query+"."+event.Command.Message["TXN"]+"", 0777)
	err = ioutil.WriteFile("./commands/"+event.Command.Query+"."+event.Command.Message["TXN"]+"/"+commandType, b, 0644)
	if err != nil {
		panic(err)
	}
}

func (tM *TheaterManager) logAnswer(msgType string, msgContent map[string]string, msgType2 uint32) {
	b, err := json.MarshalIndent(msgContent, "", "	")
	if err != nil {
		panic(err)
	}

	commandType := "answer"

	os.MkdirAll("./commands/"+msgType+"."+msgContent["TXN"]+"", 0777)
	err = ioutil.WriteFile("./commands/"+msgType+"."+msgContent["TXN"]+"/"+commandType, b, 0644)
	if err != nil {
		panic(err)
	}
}

func (tM *TheaterManager) LLST(event gs.EventClientFESLCommand) {
	if !event.Client.IsActive {
		log.Noteln("Client left")
		return
	}

	answerPacket := make(map[string]string)
	answerPacket["TID"] = event.Command.Message["TID"]
	answerPacket["NUM-LOBBIES"] = "1"
	event.Client.WriteFESL(event.Command.Query, answerPacket, 0x0)

	ldatPacket := make(map[string]string)
	ldatPacket["TID"] = "LDAT"
	ldatPacket["FAVORITE-GAMES"] = "0"
	ldatPacket["FAVORITE-PLAYERS"] = "0"
	ldatPacket["LID"] = "1"
	ldatPacket["LOCALE"] = "en_US"
	ldatPacket["MAX-GAMES"] = "10000"
	ldatPacket["NAME"] = "bfheroesPC1"
	ldatPacket["NUM-GAMES"] = "7"
	ldatPacket["PASSING"] = "7"
	event.Client.WriteFESL("LDAT", ldatPacket, 0x0)
	tM.logAnswer("LDAT", ldatPacket, 0x0)
}

func (tM *TheaterManager) USER(event gs.EventClientFESLCommand) {
	if !event.Client.IsActive {
		log.Noteln("Client left")
		return
	}

	answerPacket := make(map[string]string)
	answerPacket["TID"] = event.Command.Message["TID"]
	answerPacket["NAME"] = "Spencer"
	answerPacket["CID"] = "158"
	event.Client.WriteFESL(event.Command.Query, answerPacket, 0x0)
	tM.logAnswer(event.Command.Query, answerPacket, 0x0)
}

func (tM *TheaterManager) UBRA(event gs.EventClientFESLCommand) {
	if !event.Client.IsActive {
		log.Noteln("Client left")
		return
	}

	answerPacket := make(map[string]string)
	answerPacket["TID"] = event.Command.Message["TID"]
	event.Client.WriteFESL(event.Command.Query, answerPacket, 0x0)
	tM.logAnswer(event.Command.Query, answerPacket, 0x0)
}

func (tM *TheaterManager) UGAM(event gs.EventClientFESLCommand) {
	if !event.Client.IsActive {
		log.Noteln("Client left")
		return
	}

	log.Noteln("yo dis a server ")
	event.Client.State.IsServer = true

	gameServer := new(core.RedisState)
	gameServer.New(tM.redis, "gameServer-"+event.Command.Message["LID"])

	log.Noteln("Updating GameServer " + event.Command.Message["LID"])

	for index, value := range event.Command.Message {
		log.Noteln("SET " + index + " " + value)
		gameServer.Set(index, value)
	}
}

func (tM *TheaterManager) CONN(event gs.EventClientFESLCommand) {
	if !event.Client.IsActive {
		log.Noteln("Client left")
		return
	}

	answerPacket := make(map[string]string)
	answerPacket["TID"] = event.Command.Message["TID"]
	answerPacket["TIME"] = strconv.FormatInt(time.Now().UTC().Unix(), 10)
	answerPacket["activityTimeoutSecs"] = "15"
	answerPacket["PROT"] = event.Command.Message["PROT"]
	event.Client.WriteFESL(event.Command.Query, answerPacket, 0x0)
	tM.logAnswer(event.Command.Query, answerPacket, 0x0)
}

func (tM *TheaterManager) EGRS(event gs.EventClientFESLCommand) {
	if !event.Client.IsActive {
		return
	}

	log.Noteln("wpwww")

	answerPacket := make(map[string]string)
	answerPacket["TID"] = event.Command.Message["TID"]
	event.Client.WriteFESL("EGRS", answerPacket, 0x0)
}

func (tM *TheaterManager) newClient(event gs.EventNewClient) {
	if !event.Client.IsActive {
		log.Noteln("Client left")
		return
	}
	log.Noteln("Client connecting")

	// Start Heartbeat
	event.Client.State.HeartTicker = time.NewTicker(time.Second * 10)
	go func() {
		for {
			if !event.Client.IsActive {
				return
			}
			select {
			case <-event.Client.State.HeartTicker.C:
				if !event.Client.IsActive {
					return
				}
				pingPacket := make(map[string]string)
				pingPacket["TID"] = "0"
				event.Client.WriteFESL("PING", pingPacket, 0x0)
			}
		}
	}()
	event.Client.State.JoinTicker = time.NewTicker(time.Second * 10)
	go func() {
		for {
			if !event.Client.IsActive {
				return
			}
			select {
			case <-event.Client.State.JoinTicker.C:
				if !event.Client.IsActive {
					return
				}
				if !event.Client.State.IsServer {
					log.Noteln("Is not a server!")
				} else {

					if !wantsToJoin {
						log.Noteln("Not joining yet")
					} else {
						log.Noteln("unzipping the server's pants")
						answerPacket2 := make(map[string]string)
						answerPacket2["TID"] = "1"

						answerPacket2["NAME"] = "Spencer"
						answerPacket2["UID"] = "158"
						answerPacket2["PID"] = "100"
						answerPacket2["TICKET"] = "1234"

						answerPacket2["IP"] = "192.168.69.69"
						answerPacket2["PORT"] = localPort

						answerPacket2["INT-IP"] = "192.168.69.69"
						answerPacket2["INT-PORT"] = localPort

				
						answerPacket2["PTYPE"] = "P"

						answerPacket2["XUID"] = "24"
						answerPacket2["R-XUID"] = "24"


						answerPacket2["GROUP.[]"] = "0"

						answerPacket2["R-USER"] = "mDaWg"
						answerPacket2["R-GROUPID"] = "0"

						answerPacket2["R-U-accid"] = "100"
						answerPacket2["R-U-elo"] = "1000"
						answerPacket2["R-U-team"] = "1"
						answerPacket2["R-U-kit"] = "1"
						answerPacket2["R-U-lvl"] = "1"
						answerPacket2["R-U-dataCenter"] = "iad"
						answerPacket2["R-U-externalIp"] = "192.168.69.69"
						answerPacket2["R-U-internalIp"] = "192.168.69.69"
						answerPacket2["R-U-category"] = "3"
						
						
						answerPacket2["LID"] = "1"
						answerPacket2["GID"] = "1"
						event.Client.WriteFESL("EGRQ", answerPacket2, 0x0)
						tM.logAnswer("EGRQ", answerPacket2, 0x0)
					}
				}
			}
		}
	}()
}

func (tM *TheaterManager) close(event gs.EventClientTLSClose) {
	log.Noteln("Client closed.")

	if !event.Client.State.HasLogin {
		return
	}

}

func (tM *TheaterManager) error(event gs.EventClientTLSError) {
	log.Noteln("Client threw an error: ", event.Error)
}
