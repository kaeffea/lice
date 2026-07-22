# Ambiente local no Ubuntu WSL

Este ambiente reproduz a primeira fatia vertical do LICE sem publicar bancos ou
serviços internos na máquina. Ele é exclusivo para desenvolvimento: Keycloak usa
`start-dev`, HTTP e dados sintéticos. Não reutilize esta composição em produção.

O recorte permite validar o login do operador global, a negação de uma identidade
conhecida sem concessão, a sessão e o logout locais e a consulta visual da
auditoria. Tenants, universidades e domínio acadêmico ainda não fazem parte desta
composição.

## Pré-requisitos

- Ubuntu no WSL 2, com o repositório dentro do filesystem Linux, por exemplo
  `~/code/lice` — evite `/mnt/c`;
- Docker Desktop com integração para a distribuição Ubuntu, ou Docker Engine no
  próprio WSL;
- Docker Compose v2, GNU Make, OpenSSL, Python 3 e curl.

Confirme o ambiente com:

```bash
make doctor
```

## Primeira execução

Gere os segredos locais e suba a composição:

```bash
make dev-env
make dev
```

`make dev-env` cria `~/.config/lice/dev.env` com diretório `0700` e arquivo
`0600`. O comando é idempotente: ele valida um arquivo existente e nunca o
sobrescreve. Os valores não são impressos. Não mova o arquivo para o repositório
e não o coloque sob `/mnt/c`, onde as permissões POSIX podem não proteger os
segredos como esperado.

Depois que todos os healthchecks e one-shots concluírem:

- aplicação: <http://lice.localhost:8080>;
- provedor de identidade: <http://auth.lice.localhost:8080>;
- discovery OIDC:
  <http://auth.lice.localhost:8080/realms/lice/.well-known/openid-configuration>.

Somente o Caddy publica uma porta, em `127.0.0.1:8080`. API, web, Keycloak e
PostgreSQL ficam acessíveis apenas na rede do Compose. Dentro dessa rede,
`auth.lice.localhost` é um alias direto do Keycloak; fora dela, o mesmo hostname
chega ao Keycloak pelo Caddy. Assim navegador, API e seeder usam o mesmo issuer
público exato. Não configure a API com `http://keycloak:8080` como issuer.

As credenciais sintéticas aparecem somente quando solicitadas explicitamente:

```bash
make dev-credentials
```

Há um operador global e um usuário autenticável sem concessão. O segundo existe
para provar a negação por padrão.

O service account administrativo usado pelo seeder também é sintético,
temporário e restrito a este ambiente local. Ele permanece no banco do Keycloak
para que novas execuções do seed sejam idempotentes. Não é uma credencial nem
uma topologia aceitável para produção.

## Roteiro manual da fatia

1. Abra <http://lice.localhost:8080/entrar> e selecione **Entrar com minha
   conta**.
2. Use a credencial sintética do operador global. O resultado esperado é o
   console em `/controle`, sem conteúdo protegido exibido antes da validação da
   sessão.
3. Abra **Auditoria**, filtre ou pesquise os eventos e consulte o detalhe de uma
   sessão iniciada.
4. Encerre a sessão pelo botão **Sair**. O cookie local deixa de autorizar
   imediatamente; entre novamente como operador para consultar o evento de
   encerramento.
5. Encerre a sessão e entre com o usuário sem concessão. O resultado esperado é
   `/acesso/negado`, sem criação de sessão do LICE. Ao retornar como operador, a
   negação estará disponível na auditoria.

As credenciais são obtidas apenas por `make dev-credentials`; elas não são
versionadas nem repetidas neste roteiro.

## Ordem de inicialização

```text
PostgreSQL saudável
├── migrations concluídas
└── Keycloak saudável
    └── seed de usuários concluído
        └── subjects imutáveis gravados em volume efêmero
            └── bootstrap LICE concluído
                └── API saudável → web saudável → Caddy
```

O realm versionado não contém usuários nem segredos. O seeder cria ou encontra
os usuários sintéticos no Keycloak, obtém seus IDs imutáveis e grava apenas os
subjects em um volume compartilhado. O processo seguinte recebe esses subjects
e executa `lice-admin bootstrap-demo`; username e e-mail não estão disponíveis
para a decisão de privilégio.

O PostgreSQL local compartilha uma instância para economizar recursos, mas mantém
bases e logins separados:

| Credencial | Base acessível | Uso |
| --- | --- | --- |
| `keycloak` | `keycloak` | estado privado do provedor de identidade |
| `lice_migrator` | `lice` | DDL, migrations e bootstrap administrativo |
| `lice_runtime` | `lice` | execução da API com privilégios mínimos |

O acesso `PUBLIC` aos bancos e schemas é revogado. A credencial de runtime não é
dona das tabelas, não possui `BYPASSRLS` e só insere/lê auditoria conforme as
migrations da aplicação.

## Operação diária

```bash
make dev-status
make test
make smoke
make test-integration
make dev-logs
make dev-down
```

`make test` valida arquivos declarativos e executa, em containers, análise
estática, testes unitários e de concorrência e builds da API e da interface.
`make smoke` e `make test-integration` exigem a composição iniciada; o segundo
também prova as fronteiras entre credenciais do PostgreSQL e executa os testes da
API contra um banco real. `make dev-down` encerra os serviços, mas preserva os
volumes.

`make validate` verifica shell, JSON e `docker compose config --quiet`. Não use
`docker compose config` sem `--quiet` em capturas ou tickets: a saída resolvida
pode conter os segredos interpolados.

Para apagar dados locais, a confirmação é deliberadamente explícita:

```bash
make dev-reset CONFIRM=lice-local-data
```

O reset remove, em conjunto, o banco do Keycloak, o banco do LICE e o volume de
subjects. Apagar só o banco do Keycloak geraria novos subjects e invalidaria os
vínculos existentes no LICE.

## Limites antes de produção

A existência de `LICE_ENVIRONMENT=production` e das validações de HTTPS/cookies seguros
serve para impedir uma configuração obviamente fraca; ela não torna esta
composição uma distribuição de produção. Antes de operar dados reais ainda serão
necessários, no mínimo:

- Keycloak em modo de produção, com endurecimento, MFA conforme política,
  atualização e alta disponibilidade definidas;
- TLS, DNS e proteção de borda operados fora da composição de desenvolvimento;
- segredos em serviço apropriado, com rotação e procedimento de recuperação;
- backups e restaurações exercitados para LICE e IdP;
- monitoramento, alertas, retenção de auditoria e resposta a incidentes;
- limites distribuídos contra abuso e uma avaliação de segurança independente.

O limitador de início de login nesta fatia vive na memória de cada processo da
API. Ele reduz abuso acidental no ambiente atual, mas não substitui os controles
da borda em uma implantação com réplicas.

## Solução de problemas

- Se `make doctor` não alcançar o daemon, habilite a integração WSL no Docker
  Desktop e execute novamente dentro do Ubuntu.
- O import de realm do Keycloak cria um realm ausente, mas não reconcilia um
  realm já persistido. Depois de alterar o JSON do realm, use o reset local.
- Se a porta estiver ocupada, encerre o processo em `127.0.0.1:8080`; não altere
  apenas uma das URLs OIDC, pois issuer, callback e client precisam coincidir.
- Se as credenciais do arquivo local forem alteradas após a criação dos volumes,
  faça o reset completo para que os logins do PostgreSQL sejam recriados.

Referências operacionais: [ordem de inicialização do Docker
Compose](https://docs.docker.com/compose/how-tos/startup-order/), [importação de
realm](https://www.keycloak.org/server/importExport), [healthchecks do
Keycloak](https://www.keycloak.org/observability/health), [hostname e
proxy](https://www.keycloak.org/server/hostname) e [reverse proxy do
Caddy](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy).
