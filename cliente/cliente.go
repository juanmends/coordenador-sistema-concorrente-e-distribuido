package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func enviar_mensagem(conn net.Conn, tipo_msg string, IDprocesso int) {
	var IDmsg string
	switch tipo_msg {
	case "REQUEST":
		IDmsg = "1"
	case "RELEASE":
		IDmsg = "3"
	}

	msg := fmt.Sprintf("%s|%d|000000", IDmsg, IDprocesso)

	conn.Write([]byte(msg))
}

func aguardar_grant(conn net.Conn) error {
	buffer := make([]byte, 10)

	n, err := conn.Read(buffer)
	if err != nil {
		return fmt.Errorf("Erro ao ler %v", err)
	}

	msg := strings.TrimRight(string(buffer[:n]), "0")

	partes := strings.Split(msg, "|")

	if len(partes) < 1 || partes[0] != "2" {
		return fmt.Errorf("Mensagem não é um GRANT %v", msg)
	} else {
		fmt.Printf("GRANT recebido!")
		return nil
	}

}

func regiao_critica(IDprocesso int) {
	file, err_arq := os.OpenFile("resultado.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err_arq != nil {
		log.Println(err_arq)
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	linha := fmt.Sprintf("%d %s\n", IDprocesso, timestamp)
	file.WriteString(linha)

	file.Close()
	time.Sleep(2 * time.Second)
}

func main() {

	if len(os.Args) < 2 {
		log.Fatal("Uso: ./processo <ID>")
	}

	IDprocesso, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatal("ID deve ser um número")
	}

	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		log.Fatal("Erro ao conectar", err)
	}

	defer conn.Close()

	r := 5
	for i := 0; i < r; i++ {

		enviar_mensagem(conn, "REQUEST", IDprocesso)

		resposta := aguardar_grant(conn)

		if resposta != nil {
			log.Printf("Erro ao aguardar GRANT: %v", resposta)
			continue
		}

		regiao_critica(IDprocesso)

		enviar_mensagem(conn, "RELEASE", IDprocesso)
	}
}
