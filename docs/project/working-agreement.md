# Acordo de trabalho

Este acordo define como o LICE será planejado, implementado e validado pela colaboração entre IA e usuário. O objetivo é entregar passos pequenos, completos e verificáveis, sem confundir intenção com software funcionando.

## Unidade de entrega

- **Milestone:** resultado demonstrável que reúne uma ou mais fatias verticais relacionadas.
- **História de usuário:** necessidade observável de uma pessoa, com critérios de aceite testáveis.
- **Fatia vertical:** menor incremento que atravessa as camadas necessárias para funcionar de ponta a ponta. Inclui interface, regras, persistência, autorização, auditoria e testes quando forem aplicáveis.
- **Decisão:** escolha de produto ou arquitetura que merece contexto, alternativas e consequências registradas.

Uma fatia deve ser pequena o bastante para ser implementada, demonstrada e validada antes de avançarmos para a próxima. Tarefas internas podem existir como checklist ou subissues, mas não são entregas isoladas para o usuário.

## Fluxo

| Estado | Significado |
| --- | --- |
| `Backlog` | Ideia registrada, ainda não comprometida. |
| `Ready` | Escopo, regras e critérios de aceite estão claros. |
| `Em implementação` | Existe trabalho ativo na fatia. |
| `Aguardando validação` | Implementação e verificações automatizadas terminaram; há instruções e evidências para validação do usuário. |
| `Done` | O usuário validou a fatia e a definição de pronto foi atendida. |

`Bloqueado` é um sinal adicional, acompanhado do impedimento concreto e da decisão necessária. Só uma fatia deve permanecer em implementação por vez. A próxima fatia começa depois da validação da atual, salvo acordo explícito.

No GitHub Project, o campo personalizado `Etapa` é a fonte de verdade desse fluxo. O campo `Status` criado automaticamente pelo GitHub não será usado para declarar andamento ou conclusão.

## Ciclo de uma fatia

1. Registrar a história, o limite de escopo, as regras e os critérios de aceite.
2. Resolver decisões indispensáveis antes de escrever código dependente delas.
3. Implementar em ciclos curtos, usando testes primeiro para regras de negócio.
4. Executar as verificações relevantes e registrar resultados reproduzíveis.
5. Entregar uma demonstração com passos objetivos de validação.
6. Corrigir o que for identificado e repetir a validação.
7. Marcar como `Done` somente após a aprovação do usuário.

## Definição de pronto

Uma história está pronta quando, no que for aplicável:

- todos os critérios de aceite foram demonstrados;
- as regras de negócio possuem testes automatizados, inclusive casos de erro e limites relevantes;
- testes de integração ou de jornada cobrem os riscos que não cabem em testes unitários;
- autorização, isolamento entre tenants, proteção de dados e auditoria foram verificados;
- ações sensíveis geram eventos de auditoria úteis e estes aparecem para os perfis gerenciais autorizados;
- migrações e contratos são compatíveis com o ambiente suportado;
- documentação afetada descreve o comportamento real;
- CI e verificações locais relevantes passam;
- não restaram pendências ocultas dentro do escopo aceito;
- o usuário executou o roteiro de validação e aprovou o resultado.

## Planejado não é implementado

- Roadmaps, issues e documentos de visão descrevem intenção e devem usar linguagem como “planejado” ou “proposto”.
- README, documentação de uso e notas de versão só podem afirmar capacidades já entregues.
- Uma decisão aceita autoriza uma direção; ela não prova que a implementação existe.
- Toda afirmação de implementação deve apontar para código, teste, demonstração ou outra evidência verificável.
- Trabalho parcial permanece em `Em implementação` ou `Aguardando validação`, nunca em `Done`.

## Estratégia de testes

TDD é prioritário nas regras de domínio: escrever um exemplo que falha, implementar o mínimo para fazê-lo passar e refatorar mantendo o comportamento. Ele não precisa ser aplicado mecanicamente a estilos, arquivos declarativos ou experimentos descartáveis.

Usaremos o teste mais barato que forneça confiança suficiente:

- unitário para regras e estados;
- integração para banco, transações, isolamento e serviços externos;
- jornada para fluxos críticos vistos pelo usuário;
- carga, segurança e recuperação quando o risco da fatia justificar.

Quantidade de testes não substitui cenários significativos. Falhas encontradas na validação devem se transformar em teste de regressão sempre que isso for viável.

## Registros objetivos

Issues e pull requests devem conter somente contexto necessário, critérios, decisões, evidências e pendências reais. Atualizações descrevem o que mudou e o que foi comprovado; não repetem planos nem tratam comandos executados como resultado de produto.
