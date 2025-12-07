# Número de processos
n=5

echo "Iniciando $n processos..."

# Iniciar processos em background
for i in $(seq 1 $n)
do
    ./cliente $i &
    echo "Processo $i iniciado (PID: $!)"
done

echo "Todos os processos iniciados!"
echo "Aguardando conclusão..."

# Aguardar todos terminarem
wait

echo "Todos os processos finalizaram!"