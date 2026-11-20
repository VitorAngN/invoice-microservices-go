#!/usr/bin/env bash
# Teste ponta a ponta contra os serviços rodando com PostgreSQL real.
# Uso: STOCK_URL=http://localhost:8081 INVOICE_URL=http://localhost:8082 bash scripts/smoke-test.sh
set -uo pipefail
STOCK=${STOCK_URL:-http://localhost:8081}
INV=${INVOICE_URL:-http://localhost:8082}
JSON=(-H 'Content-Type: application/json')
falhas=0
S=$(date +%s)
P="E2E-$S"

check() { # valor_obtido valor_esperado descricao
    if [ "$1" = "$2" ]; then echo "✔ $3"; else echo "✘ $3 (esperado $2, veio $1)"; falhas=$((falhas + 1)); fi
}
code() { curl -s -o /tmp/body -w '%{http_code}' "$@"; }
saldo() { curl -s "$STOCK/api/products" | jq -r ".[] | select(.code==\"$P\") | .balance"; }
status_nota() { curl -s "$INV/api/invoices" | jq -r ".[] | select(.id==$1) | .status"; }
nova_nota() { # quantidade -> imprime o id
    curl -s "${JSON[@]}" -X POST "$INV/api/invoices" -d "{\"items\":[{\"product_code\":\"$P\",\"quantity\":$1}]}" | jq -r .id
}

check "$(code "$STOCK/health")" 200 "estoque: /health"
check "$(code "$INV/health")" 200 "faturamento: /health"

check "$(code "${JSON[@]}" -X POST "$STOCK/api/products" -d "{\"code\":\"$P\",\"description\":\"Parafuso\",\"balance\":10}")" 201 "cria produto com saldo 10"
check "$(code "${JSON[@]}" -X POST "$STOCK/api/products" -d "{\"code\":\"$P\",\"description\":\"Outro\",\"balance\":1}")" 409 "código duplicado é recusado (409)"

check "$(code "${JSON[@]}" -X POST "$INV/api/invoices" -d "{\"items\":[{\"product_code\":\"$P\",\"quantity\":3}]}")" 201 "cria nota com 3 unidades"
ID=$(jq -r .id /tmp/body)
check "$(jq -r .status /tmp/body)" Aberta "nota começa com status Aberta"
check "$(code -X POST "$INV/api/invoices/$ID/print")" 200 "imprime a nota"
check "$(status_nota "$ID")" Fechada "nota impressa fica Fechada"
check "$(saldo)" 7 "estoque baixou de 10 para 7"
check "$(code -X POST "$INV/api/invoices/$ID/print")" 409 "segunda impressão é recusada (409)"
check "$(saldo)" 7 "estoque não foi debitado duas vezes"

ID2=$(nova_nota 50)
check "$(code -X POST "$INV/api/invoices/$ID2/print")" 409 "saldo insuficiente é recusado (409)"
check "$(status_nota "$ID2")" Aberta "nota recusada volta para Aberta"
check "$(saldo)" 7 "estoque intacto após recusa"

for _ in 1 2 3; do
    curl -s -o /dev/null "${JSON[@]}" -H "Idempotency-Key: e2e-$S" -X POST "$STOCK/api/products/decrement" \
        -d "{\"items\":[{\"product_code\":\"$P\",\"quantity\":2}]}"
done
check "$(saldo)" 5 "mesma Idempotency-Key 3x debita só uma vez (7 → 5)"

ID3=$(nova_nota 1)
for i in 1 2 3 4 5 6; do
    curl -s -o /dev/null -w '%{http_code}\n' -X POST "$INV/api/invoices/$ID3/print" > "/tmp/print_$i" &
done
wait
check "$(cat /tmp/print_* | grep -c '^200$')" 1 "6 cliques simultâneos em imprimir: só 1 sucesso"
check "$(saldo)" 4 "cliques simultâneos debitam uma única vez (5 → 4)"

ID4=$(nova_nota 1)
check "$(code -H 'X-Simulate-Failure: true' -X POST "$INV/api/invoices/$ID4/print")" 503 "estoque fora do ar: retry e fallback 503"
check "$(status_nota "$ID4")" Aberta "após a falha a nota volta para Aberta"
check "$(saldo)" 4 "falha não debitou o estoque"

echo
if [ "$falhas" -eq 0 ]; then echo "Tudo certo."; else echo "$falhas verificação(ões) falharam."; fi
exit "$falhas"
