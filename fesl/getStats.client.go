package fesl

import (
	"strconv"

	"github.com/SpencerSharkey/GoFesl/GameSpy"
	"github.com/SpencerSharkey/GoFesl/log"
)

// GetStats - Get basic stats about a soldier/owner (account holder)
func (fM *FeslManager) GetStats(event GameSpy.EventClientTLSCommand) {
	if !event.Client.IsActive {
		log.Noteln("Client left")
		return
	}

	owner := event.Command.Message["owner"]
	userId := event.Client.RedisState.Get("uID")
	log.Noteln(event.Command.Message["owner"])

	loginPacket := make(map[string]string)
	loginPacket["TXN"] = "GetStats"
	loginPacket["ownerId"] = owner
	loginPacket["ownerType"] = "1"

	// Generate our argument list for the statement -> heroID, userID, key1, key2, key3, ...
	var args []interface{}
	statsKeys := make(map[string]string)
	args = append(args, owner)
	args = append(args, userId)
	keys, _ := strconv.Atoi(event.Command.Message["keys.[]"])
	for i := 0; i < keys; i++ {
		args = append(args, event.Command.Message["keys."+strconv.Itoa(i)+""])
		statsKeys[event.Command.Message["keys."+strconv.Itoa(i)+""]] = strconv.Itoa(i)
	}

	rows, err := fM.getStatsStatement(keys).Query(args...)
	if err != nil {
		log.Errorln("Failed gettings stats for hero "+owner, err.Error())
	}

	count := 0
	for rows.Next() {
		var userID, heroID, statsKey, statsValue string
		err := rows.Scan(&userID, &heroID, &statsKey, &statsValue)
		if err != nil {
			log.Errorln("Issue with database:", err.Error())
		}

		loginPacket["stats."+strconv.Itoa(count)+".key"] = statsKey
		loginPacket["stats."+strconv.Itoa(count)+".value"] = statsValue
		loginPacket["stats."+strconv.Itoa(count)+".text"] = statsValue

		delete(statsKeys, statsKey)
		count++
	}

	// Send stats not found with default value of 0
	for key := range statsKeys {
		loginPacket["stats."+strconv.Itoa(count)+".key"] = key
		loginPacket["stats."+strconv.Itoa(count)+".value"] = "0"
		loginPacket["stats."+strconv.Itoa(count)+".text"] = "0"

		count++
	}
	loginPacket["stats.[]"] = strconv.Itoa(count)

	event.Client.WriteFESL(event.Command.Query, loginPacket, event.Command.PayloadID)
	fM.logAnswer(event.Command.Query, loginPacket, event.Command.PayloadID)

}
