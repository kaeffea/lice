# LICE

**Lócus Integrado de Campus e Ensino**

Plataforma de gestão universitária em desenvolvimento público, concebida para evoluir de um fluxo profundo de matrícula para um ERP universitário completo, configurável e portável.

> **Estado atual:** a primeira fatia vertical já possui uma versão funcional para validação local, exclusivamente com dados sintéticos. Ainda não existe uma implantação pronta para uso institucional.

## Disponível agora

A fatia de acesso do operador global entrega, de ponta a ponta:

- entrada pela interface e autenticação OIDC com Keycloak;
- sessão opaca mantida no servidor, expiração por inatividade e logout;
- autorização do papel `platform_operator`, reavaliada nas requisições protegidas;
- negação segura para uma identidade conhecida sem concessão;
- console protegido com consulta, filtros e detalhe de eventos de auditoria;
- ambiente local reproduzível no Ubuntu WSL e testes automatizados.

Ela ainda não contém universidades, tenants ou dados acadêmicos.

## Executar no Ubuntu WSL

Com o repositório no filesystem Linux, por exemplo em `~/code/lice`:

```bash
make doctor
make dev-env
make dev
make dev-credentials
```

Abra <http://lice.localhost:8080>. O último comando exibe, somente sob demanda,
as duas credenciais sintéticas usadas para validar o acesso permitido e o
negado. O roteiro completo está em [Ambiente local no Ubuntu
WSL](docs/development/local.md).

Depois de iniciar o ambiente, as verificações principais são:

```bash
make test
make smoke
make test-integration
```

## Próximo marco do produto

O próximo marco demonstrável está planejado para cobrir, com dados sintéticos,
a jornada de matrícula regular:

- provisionamento de uma instituição e de seu administrador inicial;
- identidade, múltiplos vínculos e autorização com escopo;
- estrutura acadêmica e calendário mínimos;
- currículo, componentes, turmas, vagas e reservas;
- solicitação, classificação determinística, processamento e publicação do resultado;
- isolamento multi-tenant, auditoria visível e testes das regras críticas.

O prazo-alvo deste marco é **29 de julho de 2026**. Funcionalidades fora desse recorte aparecem como planejadas, nunca como implementadas.

## Limites atuais

- A composição local usa HTTP e Keycloak em `start-dev`; não deve receber dados
  pessoais, acadêmicos ou institucionais reais.
- O modo de configuração de produção exige HTTPS e cookies seguros, mas isso não
  equivale a uma implantação de produção pronta.
- Alta disponibilidade, backups e restauração testados, gestão externa e rotação
  de segredos, endurecimento do IdP e da borda, monitoramento operacional e
  avaliação de segurança ainda são trabalho futuro.
- Os modelos SaaS, dedicado e self-hosted são direções arquiteturais; seus
  pacotes e procedimentos operacionais ainda não foram entregues.

## Princípios

- construir fatias verticais pequenas e validá-las antes da próxima;
- distinguir pessoa, identidade, vínculo, função e permissão;
- tratar auditoria como funcionalidade do produto;
- preservar regras, configurações e fatos acadêmicos historicamente;
- permitir SaaS compartilhado, ambiente dedicado e instalação self-hosted sem forks do produto;
- introduzir tecnologia quando houver uma necessidade concreta e verificável.

## Documentação

- [Visão do produto](docs/product/vision.md)
- [Escopo do MVP](docs/product/mvp.md)
- [Acordo de trabalho e definição de pronto](docs/project/working-agreement.md)
- [ADR 0001 — modelos de implantação](docs/architecture/adr/0001-deployment-models.md)
- [ADR 0002 — auditoria como funcionalidade](docs/architecture/adr/0002-audit-as-product-feature.md)
- [ADR 0003 — autenticação, identidade e sessão](docs/architecture/adr/0003-authentication-identity-and-session.md)
- [ADR 0004 — fundação da aplicação web](docs/architecture/adr/0004-web-application-foundation.md)
- [Fundação de identidade da primeira fatia](docs/architecture/security-foundation.md)
- [Ambiente local no Ubuntu WSL](docs/development/local.md)

## Licença

A licença ainda não foi definida. Tornar o repositório público não concede, por si só, permissão de uso, modificação ou redistribuição.
