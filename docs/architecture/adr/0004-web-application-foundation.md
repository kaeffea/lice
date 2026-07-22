# ADR 0004 — Fundação da aplicação web

- **Status:** Aceita
- **Data:** 2026-07-15

## Contexto

A primeira fatia precisa oferecer entrada pública, estados de acesso, um shell protegido e auditoria visível. A ADR 0003 já define que a API Go será o cliente OIDC confidencial e que não haverá um BFF separado no MVP. O navegador não pode receber tokens do provedor nem renderizar dados protegidos antes da autorização.

O produto é uma aplicação transacional autenticada. Nesta etapa ele não precisa de SEO, geração de conteúdo público, Server Components ou renderização distribuída entre dois backends.

## Decisão

A interface será uma aplicação React com TypeScript estrito, compilada pelo Vite para arquivos estáticos. Caddy publicará interface e API sob a mesma origem; a API Go continuará sendo o único backend e a única parte que conhece a sessão.

O HTML inicial não conterá dados protegidos. Em rotas do plano de controle, a interface mostrará apenas um estado neutro de verificação enquanto consulta `/api/v1/session`. O shell e suas consultas serão montados somente depois de uma resposta autorizada. Negação, expiração e indisponibilidade usarão estados fechados definidos pelo produto, nunca texto refletido de parâmetros ou erros internos.

Serão usados:

- React e TypeScript;
- Vite somente para desenvolvimento, testes e build;
- CSS próprio com variáveis e escopo por componente quando útil;
- HTML semântico e alvo WCAG 2.2 AA;
- Vitest e Testing Library para comportamento de componentes;
- Playwright para as jornadas reais no navegador.

Não haverá kit visual, Tailwind, fonte externa, renderizador Node em produção nem biblioteca de estado global nesta fatia. Dependências adicionais exigirão uma necessidade observável.

### Segurança de entrega

- Interface e API serão same-origin e não habilitarão CORS genérico.
- A política de conteúdo permitirá apenas recursos empacotados pelo produto.
- Dados de sessão e auditoria usarão `Cache-Control: no-store`.
- Logout será `POST`, com validação de origem e token CSRF.
- Código de autorização poderá aparecer brevemente no callback OIDC; access token, refresh token e ID token nunca irão para URL, DOM ou armazenamento do navegador.
- Voltar pelo histórico após logout não restaurará conteúdo, porque a interface revalidará a sessão e respostas protegidas não serão armazenadas.

### Direção visual inicial

A identidade será institucional contemporânea, sóbria e acessível. A marca será textual, a tipografia usará a pilha do sistema e nenhum recurso dependerá de rede externa. A tela do Keycloak herdará o tema mantido pelo provedor e será ajustada por CSS e mensagens, sem copiar templates Freemarker.

O plano de controle mostrará somente funções reais da fatia. Recursos futuros não aparecerão como cartões, métricas ou botões inativos.

## Alternativas consideradas

### Next.js

Server-side rendering impediria qualquer flash de conteúdo e seria adequado se a interface precisasse de um backend web próprio. Foi recusado agora porque introduziria um segundo processo de servidor e uma nova fronteira de sessão sem necessidade, contrariando a decisão de a API Go cumprir esse papel no MVP.

### HTML renderizado pela API Go

Reduziria o runtime ainda mais, mas tornaria a evolução de uma interface rica de ERP menos produtiva e ofereceria menor aderência ao ecossistema que será usado nas jornadas acadêmicas.

### Biblioteca completa de componentes

Aceleraria telas grandes, mas a primeira fatia possui poucos elementos. Adotá-la agora criaria linguagem visual e dependência antes de conhecermos os padrões recorrentes do produto.

## Consequências

### Positivas

- somente um backend processa identidade, sessão e autorização;
- a imagem web de produção contém apenas arquivos estáticos;
- a aplicação pode crescer em componentes sem acoplar regras de negócio ao frontend;
- o mesmo artefato funciona em SaaS, ambiente dedicado e instalação self-hosted.

### Custos e limites

- rotas protegidas exibem brevemente um estado de verificação, pois a autorização acontece após carregar o JavaScript;
- JavaScript desabilitado não terá uma jornada operacional no MVP;
- metadados públicos da interface continuam visíveis no bundle, embora nenhum dado protegido esteja nele;
- uma necessidade futura de SSR ou renderização no servidor exigirá nova decisão.
