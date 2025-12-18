
package main

import (
	"bufio"
	"container/list"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"math/rand"
)

/////STRUCTS

type Fila struct {
	lista *list.List
	mu    sync.RWMutex
}

type Registro_Log struct {
	time       time.Time
	tipo_msg   string
	IDprocesso int
	direcao    string
}

/////VARIÁVEIS GLOBAIS

var (
	// Fila de requisições
	fila *Fila

	// Mapa de conexões: processID -> conexão
	connections    = make(map[int]net.Conn)
	mu_connections sync.RWMutex

	// Controle da região crítica (-1 = livre)
	processo_atual int = -1
	mu_atual       sync.Mutex

	// Estatísticas: processID -> quantidade de vezes atendido
	estatisticas    = make(map[int]int)
	mu_estatisticas sync.Mutex

	//Log de mensagens
	logs    []Registro_Log
	mu_logs sync.Mutex

	//Constantes
	F int = 10 //Tamanho da msg em bytes
)

/////CONSTRUTOR FILA

func nova_fila() *Fila {
	return &Fila{
		lista: list.New(),
	}
}

/////MÉTODOS FILA

func (f *Fila) adicionar(IDprocesso int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for e := f.lista.Front(); e != nil; e = e.Next() {
		if e.Value.(int) == IDprocesso {
			return // já está na fila
		}
	}
	f.lista.PushBack(IDprocesso)
}


func (f *Fila) remover() (int, bool) { //retorna o id do processo removido (int) e diz se removeu algum elemento (bool)
	f.mu.Lock()
	defer f.mu.Unlock()

	elem := f.lista.Front()
	if elem == nil {
		return 0, false
	}

	IDprocesso := f.lista.Remove(elem).(int)
	return IDprocesso, true
}

func (f *Fila) esta_vazia() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lista.Len() == 0
}

func (f *Fila) get_processos() []int {
	f.mu.RLock()
	defer f.mu.RUnlock()

	processos := make([]int, 0, f.lista.Len())

	for elem := f.lista.Front(); elem != nil; elem = elem.Next() {
		processos = append(processos, elem.Value.(int))
	}
	return processos
}

/////FUNÇÕES DE LOG

func registrar_log(tipo_msg string, IDprocesso int, direcao string) {
	mu_logs.Lock()
	defer mu_logs.Unlock()

	registro := Registro_Log{
		time:       time.Now(),
		tipo_msg:   tipo_msg,
		IDprocesso: IDprocesso,
		direcao:    direcao,
	}

	logs = append(logs, registro)
}

func salvar_log_em_arquivo() {
	mu_logs.Lock()
	defer mu_logs.Unlock()

	file, err := os.Create("coordenador.log")
	if err != nil {
		log.Printf("Erro ao criar log: %v", err)
		return
	}
	defer file.Close()

	file.WriteString("=================================================\n")
	file.WriteString("LOG DO COORDENADOR - Exclusão Mútua Distribuída\n")
	file.WriteString("=================================================\n\n")

	for _, entry := range logs {
		linha := fmt.Sprintf("%s | %-4s | %-7s | Processo %d\n",
			entry.time.Format("2006-01-02 15:04:05.000"),
			entry.direcao,
			entry.tipo_msg,
			entry.IDprocesso)
		file.WriteString(linha)
	}

	fmt.Println("\nLog Salvo com Sucesso!")
}

/////FUNÇÕES DE MSG

func enviar_mensagem(conn net.Conn, tipo_msg string, IDprocesso int) error {
	var IDmsg string
	switch tipo_msg {
	case "GRANT":
		IDmsg = "2"
	default:
		IDmsg = "0"
	}

	msg := fmt.Sprintf("%s|%d|", IDmsg, IDprocesso)
	for len(msg) < F {
		msg += "0"
	}

	_, err := conn.Write([]byte(msg[:F]))
	if err == nil {
		registrar_log(tipo_msg, IDprocesso, "SEND")
	}

	return err
}

func parse_mensagem(buffer []byte) (tipo_msg string, IDprocesso int, err error) {
	msg := strings.TrimRight(string(buffer), "\x000")
	partes := strings.Split(msg, "|")

	if len(partes) < 2 {
		return "", 0, fmt.Errorf("mensagem inválida: %s", msg)
	}

	IDprocesso, err = strconv.Atoi(partes[1])
	if err != nil {
		return "", 0, err
	}

	// Mapear ID para nome
	switch partes[0] {
	case "1":
		tipo_msg = "REQUEST"
	case "3":
		tipo_msg = "RELEASE"
	default:
		tipo_msg = "UNKNOWN"
	}

	return tipo_msg, IDprocesso, nil
}

func (f *Fila) remover_aleatorio() (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.lista.Len() == 0 {
		return 0, false
	}

	// índice aleatório
	idx := rand.Intn(f.lista.Len())

	elem := f.lista.Front()
	for i := 0; i < idx; i++ {
		elem = elem.Next()
	}

	IDprocesso := f.lista.Remove(elem).(int)
	return IDprocesso, true
}


/////TENTAR CONCEDER ACESSO (NOVA FUNÇÃO - CHAVE DA CORREÇÃO)
func tentar_conceder_acesso() {
	mu_atual.Lock()
	defer mu_atual.Unlock()

	// Região crítica livre e fila não vazia
	if processo_atual != -1 || fila.esta_vazia() {
		return
	}

	// 🔀 Delay aleatório (0–500 ms) para evitar ordem previsível
	delay := time.Duration(rand.Intn(500)) * time.Millisecond
	time.Sleep(delay)

	IDprocesso, ok := fila.remover_aleatorio()
	if !ok {
		return
	}

	processo_atual = IDprocesso

	mu_connections.RLock()
	conn, existe := connections[IDprocesso]
	mu_connections.RUnlock()

	if !existe {
		processo_atual = -1
		return
	}

	if err := enviar_mensagem(conn, "GRANT", IDprocesso); err != nil {
		log.Printf("Erro ao enviar GRANT para processo %d: %v", IDprocesso, err)
		processo_atual = -1
		return
	}

	mu_estatisticas.Lock()
	estatisticas[IDprocesso]++
	mu_estatisticas.Unlock()
}



/////PROCESSAR MSG

func processar_request(IDprocesso int) {
	registrar_log("REQUEST", IDprocesso, "RECV")
	
	// Adiciona na fila
	fila.adicionar(IDprocesso)
	
	// ✅ CORREÇÃO: Verifica imediatamente se pode conceder acesso
	tentar_conceder_acesso()
}

func processar_release(IDprocesso int) {
	registrar_log("RELEASE", IDprocesso, "RECV")

	mu_atual.Lock()
	if processo_atual == IDprocesso {
		processo_atual = -1
	}
	mu_atual.Unlock()

	// ✅ CORREÇÃO: Após liberar, tenta conceder para o próximo da fila
	tentar_conceder_acesso()
}

/////ATENDER PROCESSO

func atender_processo(conn net.Conn) {
	defer conn.Close()

	var IDprocesso int = -1
	primeira_msg := true

	for {
		buffer := make([]byte, F)
		n, err := conn.Read(buffer)

		if err != nil {
			if IDprocesso != -1 {
				fmt.Printf("[INFO] Processo %d desconectado\n", IDprocesso)

				mu_connections.Lock()
				delete(connections, IDprocesso)
				mu_connections.Unlock()
			} else {
				fmt.Println("[INFO] Conexão encerrada antes da identificação")
			}
			return
		}

		tipo_msg, pid, err := parse_mensagem(buffer[:n])
		if err != nil {
			log.Printf("Erro ao parsear mensagem: %v", err)
			continue
		}

		if primeira_msg {
			IDprocesso = pid
			mu_connections.Lock()
			connections[IDprocesso] = conn
			mu_connections.Unlock()
			fmt.Printf("[INFO] Processo %d conectado\n", IDprocesso)
			primeira_msg = false
		}

		switch tipo_msg {
		case "REQUEST":
			processar_request(IDprocesso)
		case "RELEASE":
			processar_release(IDprocesso)
		default:
			log.Printf("Tipo de mensagem desconhecido: %s", tipo_msg)
		}
	}
}

func interface_terminal() {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("   COORDENADOR - Exclusão Mútua Distribuída")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println("\n📋 Comandos disponíveis:")
	fmt.Println("  1 - Imprimir fila de pedidos atual")
	fmt.Println("  2 - Imprimir estatísticas de atendimento")
	fmt.Println("  3 - Salvar log e encerrar execução")
	fmt.Println(strings.Repeat("=", 50) + "\n")

	for {
		fmt.Print("coordenador> ")
		if !scanner.Scan() {
			break
		}

		comando := strings.TrimSpace(scanner.Text())

		switch comando {
		case "1":
			fmt.Println("\n" + strings.Repeat("-", 40))
			processos := fila.get_processos()
			if len(processos) == 0 {
				fmt.Println("Fila Vazia")
			} else {
				fmt.Printf("Fila (%d processos): %v\n", len(processos), processos)
			}

			mu_atual.Lock()
			if processo_atual != -1 {
				fmt.Printf("🔒 Região crítica ocupada por: Processo %d\n", processo_atual)
			} else {
				fmt.Println("🔓 Região crítica: LIVRE")
			}
			mu_atual.Unlock()
			fmt.Println(strings.Repeat("-", 40))
		case "2":
			fmt.Println("\n" + strings.Repeat("-", 40))

			mu_estatisticas.Lock()
			if len(estatisticas) == 0 {
				fmt.Println("Nenhum processo foi atendido ainda")
			} else {
				fmt.Println("Estatísticas de Atendimento:")
				fmt.Println(strings.Repeat("-", 40))
				total := 0
				for pid, count := range estatisticas {
					fmt.Printf("   Processo %d: %d vezes\n", pid, count)
					total += count
				}
				fmt.Println(strings.Repeat("-", 40))
				fmt.Printf("   Total de atendimentos: %d\n", total)
			}
			mu_estatisticas.Unlock()

			fmt.Println(strings.Repeat("-", 40))
		case "3":
			fmt.Println("\nEncerrando coordenador...")
			salvar_log_em_arquivo()
			fmt.Println("Até logo!")
			os.Exit(0)
		default:
			fmt.Println("❌ Comando inválido! Use: 1, 2 ou 3")
		}
	}

}

func main() {
	fmt.Println("Iniciando Coordenador...")
	  rand.Seed(time.Now().UnixNano())
	fila = nova_fila()

	// ✅ CORREÇÃO: Removido o processar_fila_loop() - não é mais necessário!
	// A verificação agora acontece de forma síncrona quando REQUEST chega ou RELEASE é feito

	go interface_terminal()

	sock, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		log.Fatalln("Erro ao iniciar o servidor", err)
	}

	defer sock.Close()

	fmt.Println("Coordenador rodando na porta 8080")
	fmt.Println("Aguardando conexões...")

	for {
		conn, err := sock.Accept()
		if err != nil {
			log.Println("Erro ao aceitar conexão:", err)
			continue
		}

		go atender_processo(conn)
	}
}