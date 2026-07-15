# ADR 0002 — Auditoria como funcionalidade do produto

- **Status:** Proposta (aguardando validação no PR)
- **Data:** 2026-07-15

## Contexto

O LICE executará ações de alto impacto, como atribuir papéis, configurar períodos, alterar oferta de vagas, processar matrículas e consolidar resultados acadêmicos. Gestores autorizados precisam compreender o que ocorreu sem depender de acesso ao servidor ou de uma investigação manual em logs.

Logs técnicos e auditoria funcional possuem finalidades diferentes. Logs ajudam a diagnosticar software e infraestrutura; a trilha de auditoria registra ações e decisões relevantes do domínio de maneira estável e compreensível. A [ANPD descreve controle de acesso](https://www.gov.br/anpd/pt-br/centrais-de-conteudo/materiais-educativos-e-publicacoes/guia_seguranca_da_informacao_para_atpps___defeso_eleitoral.pdf) como a combinação de autenticação, autorização e auditoria. As recomendações da [OWASP sobre registros de aplicação](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html) também orientam a evitar segredos e dados pessoais desnecessários nos registros.

## Decisão

A trilha de auditoria será uma funcionalidade explícita do produto, separada dos logs técnicos. Toda história que introduzir uma ação sensível deverá definir seus eventos auditáveis, sua visualização autorizada e seus testes.

### Estrutura do evento

Um evento de auditoria deverá registrar, quando aplicável:

- identificador, tipo e versão do evento;
- instante no servidor;
- tenant e escopo organizacional;
- ator humano ou de sistema e sua sessão técnica;
- papel e contexto utilizados pelo ator;
- ação tentada, recurso e identificador do recurso;
- resultado: sucesso, negação ou falha;
- motivo informado ou código da decisão;
- alterações permitidas, representadas por campos anteriores e posteriores;
- `correlation_id` para relacionar o evento aos logs e *traces* técnicos;
- metadados mínimos de origem necessários à investigação.

O formato será versionado. A apresentação em linguagem humana será derivada do evento estruturado; não armazenaremos apenas uma frase livre.

### Interface e autorização

A interface oferecerá uma linha do tempo e filtros por período, ator, ação, recurso, unidade e resultado. Recursos relevantes terão um atalho para seu histórico. Detalhes mostrarão contexto, justificativa e alterações somente quando o observador possuir autorização para esses dados.

As capacidades iniciais serão independentes:

- `audit.read`;
- `audit.read_sensitive`;
- `audit.export`;
- `audit.manage_retention`.

O operador global verá eventos do plano de controle, não a atividade acadêmica dos tenants. Um gestor institucional verá apenas eventos do próprio tenant e dentro de seu escopo. Ler detalhes sensíveis, exportar auditoria ou alterar uma política de retenção também será auditado.

### Persistência, atomicidade e integridade

Eventos serão *append-only* para a aplicação: poderão ser inseridos, mas não atualizados. A credencial normal da aplicação não terá permissão para alterar ou apagar eventos.

Uma mutação de negócio confirmada e seus eventos correspondentes serão persistidos na mesma transação do banco. Assim, não aceitaremos uma alteração confirmada sem sua evidência de auditoria. Quando uma ação envolver um efeito externo assíncrono, a intenção e o *outbox event* serão gravados atomicamente; a conclusão ou falha do efeito produzirá evento próprio. Tentativas negadas e falhas anteriores a uma mutação serão registradas por caminho confiável separado, pois não podem depender da transação revertida.

Idempotência será preservada: repetir a mesma requisição idempotente não poderá duplicar a mudança nem os eventos semânticos de sucesso.

### Minimização e redação

O evento usará listas permitidas de campos, não cópias integrais das requisições ou entidades. Não serão registrados:

- senhas, tokens, cookies, chaves ou segredos;
- conteúdo completo de documentos;
- documentos pessoais completos quando um identificador interno for suficiente;
- dados acadêmicos ou pessoais sem necessidade para a finalidade de auditoria;
- valores anteriores e posteriores de campos classificados como secretos.

Valores sensíveis necessários serão mascarados, resumidos ou substituídos por identificadores. Endereço IP e outros dados de origem terão finalidade e retenção definidas antes de serem incluídos. A [LGPD](https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2018/lei/l13709compilado.htm) orientará minimização, segurança, transparência e retenção, mas prazos concretos dependerão da categoria do evento e das obrigações da instituição.

### Limite da imutabilidade

*Append-only* e restrições da credencial da aplicação protegem contra alterações pelo produto e contra erros operacionais comuns. Isso não constitui imutabilidade absoluta diante de um administrador do banco ou da infraestrutura.

Assinatura encadeada, *checkpoints* externos, armazenamento WORM e retenção legal serão avaliados posteriormente. Até que existam, a interface e a documentação não descreverão a trilha como inviolável.

## Critérios da primeira fatia vertical

Na jornada em que o operador global provisiona uma universidade, a fatia só estará concluída quando:

1. criar a universidade, iniciar o convite do administrador e atribuir seu papel produzir eventos distintos e correlacionados;
2. a interface do operador exibir esses eventos em ordem, com ator, resultado, contexto e detalhes permitidos;
3. falhas e negações relevantes também forem visíveis sem expor segredos;
4. o estado de negócio e seus eventos de sucesso forem atômicos;
5. a repetição da requisição com a mesma chave idempotente não duplicar efeitos nem eventos semânticos;
6. testes provarem o isolamento entre tenants e a negação fora do escopo;
7. testes provarem a redação de campos sensíveis;
8. testes provarem que a credencial da aplicação não atualiza nem apaga eventos;
9. consultar detalhes sensíveis e exportar, quando habilitados, gerarem novos eventos de auditoria;
10. cada evento expuser um `correlation_id` utilizável para investigação técnica.

## Consequências

### Positivas

- Gestores autorizados poderão investigar mudanças pela própria interface.
- Regras de autorização e isolamento receberão evidência testável desde a primeira fatia.
- A correlação com observabilidade técnica reduzirá o tempo de diagnóstico.
- O desenho estruturado permitirá exportações e verificações futuras sem analisar texto livre.

### Custos e restrições

- Toda ação sensível exigirá modelagem, autorização, tradução visual e testes de auditoria.
- Eventos versionados precisarão permanecer legíveis após mudanças no domínio.
- Diferenças detalhadas aumentam risco de exposição e deverão passar por revisão de minimização.
- Volume, índices, particionamento e retenção terão impacto operacional e precisarão de métricas reais.

## Questões adiadas

- prazos de retenção por categoria e instituição;
- formato e autorização das exportações;
- proteção criptográfica e cópia externa dos eventos;
- tratamento de solicitações de titulares quando a trilha contiver dados pessoais;
- política para eventos produzidos durante indisponibilidade parcial.
