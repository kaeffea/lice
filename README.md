# LICE

**Lócus Integrado de Campus e Ensino**

Plataforma de gestão universitária em desenvolvimento público, concebida para evoluir de um fluxo profundo de matrícula para um ERP universitário completo, configurável e portável.

> **Estado atual:** fundação do produto. Ainda não existe uma versão funcional nem uma implantação pronta para uso institucional.

## Primeiro resultado

O primeiro marco demonstrável cobre, com dados sintéticos, a jornada de matrícula regular:

- provisionamento de uma instituição e de seu administrador inicial;
- identidade, múltiplos vínculos e autorização com escopo;
- estrutura acadêmica e calendário mínimos;
- currículo, componentes, turmas, vagas e reservas;
- solicitação, classificação determinística, processamento e publicação do resultado;
- isolamento multi-tenant, auditoria visível e testes das regras críticas.

O prazo-alvo deste marco é **29 de julho de 2026**. Funcionalidades fora desse recorte aparecem como planejadas, nunca como implementadas.

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

## Licença

A licença ainda não foi definida. Tornar o repositório público não concede, por si só, permissão de uso, modificação ou redistribuição.
