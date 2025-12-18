package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const F = 10 // tamanho fixo da mensagem

// ----------------------
// Enviar mensagem
// ----------------------
func enviar_mensagem(conn net.Conn, tipo_msg string, IDprocesso int) error {
	var IDmsg string

	switch tipo_msg {
	case "REQUEST":
		IDmsg = "1"
	case "RELEASE":
		IDmsg = "3"
	default:
		return fmt.Errorf("tipo de mensagem inválido")
	}

	msg := fmt.Sprintf("%s|%d|", IDmsg, IDprocesso)
	for len(msg) < F {
		msg += "0"
	}

	_, err := conn.Write([]byte(msg[:F]))
	return err
}

// ----------------------
// Aguardar GRANT
// ----------------------
func aguardar_grant(conn net.Conn) error {
	buffer := make([]byte, F)

	n, err := conn.Read(buffer)
	if err != nil {
		return fmt.Errorf("erro ao ler GRANT: %v", err)
	}

	msg := strings.TrimRight(string(buffer[:n]), "0")
	partes := strings.Split(msg, "|")

	if len(partes) < 1 || partes[0] != "2" {
		return fmt.Errorf("mensagem não é GRANT: %s", msg)
	}

	fmt.Println("GRANT recebido")
	return nil
}

// ----------------------
// Região crítica
// ----------------------
func regiao_critica(IDprocesso int) {
	file, err := os.OpenFile(
		"resultado.txt",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		log.Println(err)
		return
	}
	defer file.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	linha := fmt.Sprintf("Processo %d entrou na RC em %s\n", IDprocesso, timestamp)
	file.WriteString(linha)

	// Simula tempo na região crítica
	time.Sleep(2 * time.Second)
}

// ----------------------
// MAIN
// ----------------------
func main() {

	if len(os.Args) < 2 {
		log.Fatal("Uso: ./processo <ID>")
	}

	IDprocesso, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatal("ID deve ser um número")
	}

	// Inicializa aleatoriedade (ESSENCIAL)
	rand.Seed(time.Now().UnixNano())

	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		log.Fatal("Erro ao conectar ao coordenador:", err)
	}
	defer conn.Close()

	const repeticoes = 5

	for i := 0; i < repeticoes; i++ {

		// REQUEST
		if err := enviar_mensagem(conn, "REQUEST", IDprocesso); err != nil {
			log.Println("Erro ao enviar REQUEST:", err)
			continue
		}

		// Aguarda GRANT
		if err := aguardar_grant(conn); err != nil {
			log.Println(err)
			continue
		}

		// Região crítica
		regiao_critica(IDprocesso)

		// RELEASE
		if err := enviar_mensagem(conn, "RELEASE", IDprocesso); err != nil {
			log.Println("Erro ao enviar RELEASE:", err)
			continue
		}

		// 🔀 Delay aleatório ANTES de um novo REQUEST
		time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
	}
}
