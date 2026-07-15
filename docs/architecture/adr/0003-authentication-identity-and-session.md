# ADR 0003 — Autenticação, identidade e sessão

- **Status:** Aceita
- **Data:** 2026-07-15

## Contexto

O LICE precisa autenticar pessoas em uma oferta SaaS compartilhada, em ambientes dedicados e em instalações self-hosted. Ao mesmo tempo, uma pessoa pode acumular vínculos e funções, como estudante, docente, técnico ou coordenador, sem receber uma conta diferente para cada atuação.

Autenticar não é autorizar. O provedor de identidade conhece credenciais, fatores e sessões de login; o LICE conhece instituições, pessoas, vínculos, designações, escopos e regras acadêmicas. Colocar papéis acadêmicos em tokens criaria autorizações desatualizadas, difíceis de restringir por curso ou unidade e acopladas a um fornecedor.

O provedor também processa identificadores pessoais. Portanto, ele pertence ao perímetro protegido de identidade de cada instalação e não ao plano de controle sem PII definido na ADR 0001.

## Direcionadores

- padrões abertos e ausência de dependência obrigatória de uma nuvem;
- execução local reproduzível no Ubuntu WSL com Docker;
- credenciais, tokens e segredos fora do navegador e do domínio acadêmico;
- revogação e troca de contexto verificadas no servidor;
- isolamento entre tenants e mínimo privilégio;
- uma primeira fatia pequena, sem construir um sistema próprio de credenciais;
- possibilidade de federação futura com o IdP de cada instituição.

## Alternativas consideradas

| Alternativa | Benefícios | Custos e riscos | Resultado |
| --- | --- | --- | --- |
| [Keycloak](https://github.com/keycloak/keycloak) | OIDC e SAML maduros, federação, MFA, imagem oficial, execução self-hosted e licença Apache 2.0 | Consumo e operação maiores por usar JVM; atualizações e configuração exigem disciplina | Escolhida como referência inicial |
| [ZITADEL](https://github.com/zitadel/zitadel) | Modelo de organizações forte, APIs modernas, implementação em Go e opção self-hosted | Topologia operacional mais sofisticada e licença AGPL para versões atuais | Não escolhida para o MVP |
| [authentik](https://docs.goauthentik.io/install-config/install/docker-compose/) | Boa experiência administrativa, OIDC/SAML e Docker Compose | Mais componentes e menor benefício para o recorte estrito do MVP | Não escolhida para o MVP |
| [Ory Hydra + Kratos](https://www.ory.com/hydra) | Componentes especializados, portáveis e adequados a arquiteturas headless | Exige montar e operar provedor, identidade, consentimento e interfaces separadamente | Complexidade prematura |
| Serviço gerenciado ou autenticação própria | Um gerenciado acelera o início; construir oferece controle total | Dependência comercial e menor paridade self-hosted no primeiro caso; risco de segurança inaceitável no segundo | Rejeitadas como base obrigatória |

A escolha do Keycloak é uma implementação de referência, não um contrato de domínio. O contrato do LICE com o provedor será OIDC; APIs administrativas proprietárias só poderão ser usadas atrás de limites explícitos e não definirão autorização acadêmica.

## Decisão

### Provedor e topologia

O ambiente local e a demonstração usarão **[Keycloak 26.7.0](https://www.keycloak.org/2026/07/keycloak-2670-released)**, em versão fixada e nunca como `latest`. Haverá um realm do LICE por instalação, sem usar o realm administrativo `master`. No SaaS compartilhado, os tenants continuarão sendo entidades do LICE dentro de um único realm; no dedicado ou self-hosted, cada instalação possuirá seu próprio realm ou apontará futuramente para um provedor OIDC institucional compatível.

Realm, Organizations, grupos e roles do Keycloak não serão fonte de verdade para tenants nem para permissões acadêmicas. Organizations e brokering poderão ser avaliados posteriormente para descoberta e federação de IdPs institucionais, sem substituir os vínculos mantidos pelo LICE.

No desenvolvimento, a imagem oficial será executada com `start-dev --import-realm`. Esse modo não é permitido fora do ambiente local. O realm inicial, o cliente e as configurações não secretas serão versionados; usuários, senhas e segredos não serão incluídos no arquivo. O cliente será confidencial, com Authorization Code e PKCE habilitados, e com implicit flow, direct access grants e `offline_access` desabilitados. Em ambientes não locais será usado `start`, com configuração endurecida e banco próprio. O banco do Keycloak terá base e credencial distintas das usadas pela aplicação, ainda que o Docker Compose local compartilhe a mesma instância PostgreSQL.

A [documentação de containers do Keycloak](https://www.keycloak.org/server/containers) distingue a inicialização de desenvolvimento da imagem otimizada para produção. A [importação no início](https://www.keycloak.org/server/importExport) cria configurações ausentes, mas não será tratada como mecanismo de reconciliação ou backup.

### Fluxo web e sessão

A aplicação usará OpenID Connect Authorization Code com **PKCE S256**, `state` e `nonce` únicos, cliente confidencial, URIs de retorno exatas e validação de emissor, assinatura, audiência, tempo e nonce. O fluxo segue as recomendações do [OAuth 2.0 Security BCP](https://www.rfc-editor.org/rfc/rfc9700.html).

A API web do LICE, implementada em Go, será o cliente OIDC confidencial e atuará como backend da interface, sem criar um serviço BFF separado no MVP:

```text
navegador
  └── cookie opaco de sessão
      └── API web do LICE
          ├── fluxo OIDC e sessão no servidor
          ├── autorização contextual do LICE
          └── IdP OIDC
```

O navegador e o código JavaScript não receberão access tokens, refresh tokens ou ID tokens. O identificador de sessão será aleatório, terá somente seu resumo armazenado no PostgreSQL e será enviado em cookie `HttpOnly`, `SameSite=Lax`, `Path=/` e sem `Domain`. Fora do modo local, o cookie será `Secure` e usará o prefixo `__Host-`. O modo HTTP local poderá usar um nome sem esse prefixo e `Secure=false`, protegido por uma configuração que falha ao iniciar fora do ambiente local.

Requisições mutáveis também validarão origem e token CSRF. CORS não será usado para liberar origens genéricas: interface e API serão publicadas sob a mesma origem. A sessão guardará apenas referências de segurança e o mínimo criptografado necessário ao logout do provedor; tokens de acesso e refresh serão descartados após o callback enquanto não houver uma necessidade de integração aprovada.

Os padrões iniciais serão:

- transação de login de uso único, com expiração em 5 minutos;
- 30 minutos de inatividade;
- 8 horas de duração absoluta;
- sem “lembrar de mim”;
- rotação do identificador ao autenticar ou elevar o nível de segurança da sessão;
- reautenticação futura para operações de maior risco.

Esses valores poderão ser configuráveis dentro de limites seguros, mas uma instituição não poderá remover expiração nem enfraquecer invariantes da plataforma.

### Conta, identidade e pessoa

Os nomes definitivos do modelo serão confirmados no ERD, mas estes conceitos são invariantes:

```text
identidade externa (issuer + subject)
  └── conta da instalação
      ├── atribuições do plano da plataforma
      └── acesso ao tenant
          └── pessoa institucional daquele tenant
              ├── vínculos
              ├── designações temporárias
              └── concessões com capacidade, escopo e vigência
```

- `(issuer, subject)` identifica a identidade autenticada. E-mail, username, CPF e nome nunca serão chaves de conta ou critérios automáticos de união. Essa escolha segue as regras de estabilidade de identificadores do [OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html#ClaimStability).
- A conta existe somente dentro de uma instalação do LICE. No SaaS compartilhado ela poderá acessar mais de um tenant; instalações dedicadas ou self-hosted não compartilharão contas.
- A pessoa e seus dados permanecem tenant-scoped. A mesma conta pode corresponder a uma pessoa na instituição A e outra na B sem unir PII ou históricos entre controladores.
- Dentro de um tenant, uma conta ativa liga-se a no máximo uma pessoa e uma pessoa a no máximo uma conta ativa. Correções serão explícitas e auditadas.
- Uma pessoa importada pode existir sem conta. Encerrar um vínculo não apaga a pessoa, não encerra outros vínculos e não reescreve histórico.
- O modelo permitirá várias identidades externas por conta, mas o MVP operará uma identidade por conta e não oferecerá vinculação self-service.
- Não haverá autocadastro público. Convites ou bootstrap explícito iniciarão o vínculo; e-mail verificado poderá conferir um convite, mas nunca ligará contas sozinho.

### Bootstrap do primeiro operador

O primeiro usuário autenticado jamais receberá privilégios automaticamente. Um comando administrativo idempotente, executado fora da interface comum, registrará previamente o par exato `(issuer, subject)` como operador da plataforma. No ambiente de demonstração, esse valor virá de fixture versionada sem senha real; em produção, de configuração secreta e procedimento operacional auditado.

O bootstrap não concede acesso acadêmico. Depois que existir uma administração operacional segura, novos operadores serão incluídos por uma jornada autorizada, e o mecanismo inicial poderá ser desabilitado.

### Autorização e contexto de atuação

O LICE adotará autorização contextual com catálogo inicial de capacidades e conjuntos de papel definidos pelo produto. O MVP permitirá atribuir papéis, escopos e vigências, mas não editar arbitrariamente o catálogo de permissões.

Os conjuntos iniciais serão operador da plataforma, administrador do tenant, registro acadêmico, coordenação de curso, secretaria de curso e estudante. “Docente” será vínculo; poderes futuros sobre uma turma dependerão de atribuição àquela turma. Administrador do tenant não será superusuário acadêmico, e operador da plataforma não poderá consultar dados acadêmicos.

Depois do login, a API disponibilizará somente os contextos que a conta pode usar: plataforma ou um tenant e espaço de atuação. Uma pessoa com vários vínculos escolherá, por exemplo, Estudante, Coordenação ou Administração. A interface manterá por aba uma referência opaca `context_ref` e a enviará em cada operação. Essa referência não será credencial, não conterá tenant nem permissões confiáveis e só terá sentido junto da sessão autenticada.

A seleção de contexto:

- revalidará conta, tenant, acesso, vínculo, designação, vigência e escopo;
- nunca unirá silenciosamente permissões de contextos diferentes;
- produzirá auditoria e poderá coexistir com outro contexto em outra aba;
- não transportará privilégios globais para dentro de um tenant.

Toda operação será negada por padrão e reavaliada no servidor com o estado atual. Tenant ou contexto recebido da rota ou do cliente será apenas um seletor, jamais autoridade. A autorização efetiva exigirá conta e tenant ativos, acesso à pessoa correspondente, capacidade vigente e escopo compatível com o recurso. Expirar uma designação durante a sessão causará negação já na próxima operação. Decisões de autorização não serão congeladas em tokens nem armazenadas em cache no MVP.

Um gestor não poderá usar capacidades gerenciais para alterar o próprio registro acadêmico; jornadas de autosserviço serão avaliadas por políticas separadas. Escopos de unidade não herdarão toda capacidade para seus descendentes: cada capacidade declarará seu alcance.

### Isolamento multi-tenant

No SaaS compartilhado, entidades do plano de dados terão `tenant_id` obrigatório. Unicidades e referências entre essas entidades incluirão o tenant; consultas continuarão aplicando predicados explícitos de isolamento.

PostgreSQL Row-Level Security será uma defesa adicional, não o autorizador acadêmico. Cada transação do plano de dados definirá o tenant validado com `SET LOCAL`; as políticas falharão fechadas quando ele estiver ausente e cobrirão leitura e escrita com `USING` e `WITH CHECK`. Será usado `FORCE ROW LEVEL SECURITY`, e a credencial normal da aplicação não será owner das tabelas nem possuirá `BYPASSRLS`, conforme os limites documentados pelo [PostgreSQL](https://www.postgresql.org/docs/current/ddl-rowsecurity.html).

Plano de controle, identidade e plano de dados usarão credenciais ou pools com privilégios distintos. O operador global não receberá tenant curinga nem acesso ao pool acadêmico. Instalações com um único tenant manterão `tenant_id` e as mesmas invariantes, evitando um produto ou caminho de migração diferente.

### Logout, expiração e indisponibilidade

Logout revogará primeiro a sessão do LICE e removerá o cookie; reutilizar esse cookie falhará imediatamente. Em seguida, a aplicação solicitará [RP-Initiated Logout](https://openid.net/specs/openid-connect-rpinitiated-1_0.html) ao provedor. Falha ou indisponibilidade do IdP não reativará a sessão local.

O MVP não aceitará uma sessão nova sem concluir o callback e falhará fechado quando o IdP estiver indisponível. Como não haverá bearer token do usuário no navegador nem entre a interface e a API, logout e revogação locais não terão uma janela residual de JWT nesse fluxo. Back-channel logout, federação institucional e sessões para clientes não web serão decisões posteriores.

### Eventos de segurança e auditoria

O LICE produzirá eventos funcionais, com códigos de motivo e sem credenciais, tokens, claims brutas, CPF ou e-mail não confirmado:

| Evento | Momento |
| --- | --- |
| `security.session_started` | callback válido, identidade reconhecida e sessão criada |
| `security.login_rejected` | callback inválido ou identidade sem acesso é rejeitada pelo LICE |
| `security.access_denied` | pessoa autenticada tenta ação ou contexto sem capacidade |
| `security.session_expired` | expiração por inatividade ou limite absoluto é detectada |
| `security.session_ended` | logout ou revogação local conclui |
| `security.context_changed` | contexto válido é trocado |
| `security.identity_linked` / `security.identity_unlinked` | associação administrativa muda |
| `security.grant_created` / `security.grant_revoked` | concessão privilegiada muda |

Falhas de credencial anteriores ao callback pertencem inicialmente à auditoria do Keycloak; o LICE não afirmará observá-las sem integração específica. Falhas de callback e tentativas anônimas terão logs técnicos minimizados e, quando houver tenant confiável e utilidade gerencial, evento funcional sem identificador fornecido pelo atacante. As regras da ADR 0002 continuam válidas para visibilidade, redação e escopo.

### Limites de responsabilidade

| Provedor OIDC | LICE |
| --- | --- |
| credenciais, MFA, recuperação e bloqueio de autenticação | conta da instalação e estado de acesso ao produto |
| sessão do IdP e emissão/validação protocolar de tokens | sessão opaca da aplicação e proteção CSRF |
| autenticação e federação com IdPs externos | pessoas, tenants, vínculos, designações e convites |
| políticas próprias de senha e fatores | capacidades, escopos, vigências e conflitos de interesse |
| eventos internos de autenticação | auditoria funcional e visível das ações do LICE |

## Consequências

### Positivas

- O navegador não armazena tokens e o logout local é imediatamente verificável.
- Pessoas com vários vínculos usam uma conta sem somar privilégios silenciosamente.
- Permissões continuam atuais mesmo quando um vínculo expira durante uma sessão.
- Instituições podem adotar IdPs próprios futuramente sem mover regras acadêmicas para tokens.
- A separação entre conta da instalação e pessoa do tenant evita deduplicação indevida de PII.

### Custos e riscos

- Keycloak adiciona um serviço e um banco lógico que precisam de atualização, backup e monitoramento.
- Sessões server-side criam estado e exigirão estratégia de limpeza, índices e alta disponibilidade antes da produção.
- Trocar o IdP é protocolarmente possível, mas migração de credenciais e relink de identidades não são automáticos.
- Um realm compartilhado no SaaS reduz complexidade inicial, mas poderá não atender toda exigência institucional de isolamento; nesses casos serão usados os modelos dedicado ou self-hosted, ou uma decisão futura por issuer.
- Catálogo fixo de capacidades acelera e protege o MVP, porém papéis personalizados exigirão modelagem posterior.

## Impacto nas próximas histórias

A issue #3 deverá implementar e provar somente o login do operador global, o bootstrap, a sessão, a negação, o logout e a auditoria mínima usando o contrato acima. A reprodução local incluirá Keycloak e seus dados sintéticos; esta ADR, sozinha, não afirma que esse ambiente exista.

A issue #4 manterá convite, pessoa, papel e escopo no LICE. O papel privilegiado só se tornará utilizável após aceite e comprovação da identidade, e o envio externo do convite terá estados e outbox próprios.

## Questões adiadas

- federação OIDC/SAML e descoberta do IdP por instituição;
- Organizations, SCIM e APIs administrativas do Keycloak;
- MFA obrigatório, passkeys, recuperação e vinculação de contas;
- papéis e capacidades personalizados por tenant;
- contas de serviço, APIs públicas e clientes móveis;
- back-channel logout e resposta imediata a eventos de risco do IdP;
- alta disponibilidade, rotação de chaves e endurecimento de produção;
- procedimento de suporte temporário, que não existirá no MVP.
