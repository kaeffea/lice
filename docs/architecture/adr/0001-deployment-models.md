# ADR 0001 — Modelos de implantação

- **Status:** Aceita
- **Data:** 2026-07-15

## Contexto

O LICE tratará dados acadêmicos, pessoais e administrativos de instituições com diferentes requisitos de soberania, segurança, contratação e capacidade operacional. Algumas aceitarão um SaaS compartilhado; outras exigirão isolamento de infraestrutura ou execução sob seu próprio controle.

Manter produtos distintos para cada cenário fragmentaria o código, as migrações e os controles de segurança. Ao mesmo tempo, obrigar todas as instituições a enviarem seus dados a uma infraestrutura central inviabilizaria adoções que exigem controle institucional do ambiente.

A [LGPD](https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2018/lei/l13709compilado.htm) exige medidas técnicas e administrativas adequadas ao tratamento de dados pessoais, mas não impõe, por si só, um único modelo de hospedagem. Para órgãos do SISP, a [Portaria SGD/MGI nº 5.950/2023](https://www.gov.br/governodigital/pt-br/contratacoes-de-tic/legislacao/modelo-de-contratacao-de-software-e-servicos-em-nuvem/vigentes/modelo-de-contratacao-de-software-e-nuvem/portaria-sgd-mgi-no-5-950-de-26-de-outubro-de-2023) determina que a estratégia de nuvem considere classificação da informação, segurança, privacidade e requisitos de cada carga de trabalho. Informações formalmente classificadas possuem exigências próprias; a [IN GSI/PR nº 8/2025](https://www.gov.br/gsi/pt-br/centrais-de-conteudo/noticias/2025/nota-tecnica-sobre-a-instrucao-normativa-gsi-no-8-2025-e-acordo-de-cooperacao-tecnica) não deve ser confundida com a disciplina geral de dados pessoais.

## Decisão

O LICE será **um único produto**, distribuído a partir do mesmo código e dos mesmos artefatos versionados, com três modelos de implantação:

| Modelo | Infraestrutura e banco de dados | Operação principal |
| --- | --- | --- |
| SaaS compartilhado | Mantidos pelo provedor do LICE, com isolamento multi-tenant | Provedor do LICE |
| Dedicado/BYOC (*bring your own cloud*) | Conta, projeto ou assinatura controlada pela instituição, com recursos dedicados | Provedor do LICE, dentro dos limites autorizados pela instituição |
| Self-hosted | Datacenter ou nuvem escolhida e controlada pela instituição | Instituição ou parceiro por ela contratado |

A diferença entre os modelos será expressa por configuração, topologia e automação de implantação, não por *forks* permanentes do produto.

### Planos de controle e de dados

Separaremos conceitualmente:

- **plano de controle:** catálogo de instalações, versão implantada, recursos habilitados, estado técnico e coordenação de atualizações;
- **plano de dados:** pessoas, vínculos, cursos, matrículas, notas, documentos, arquivos e trilha de auditoria da instituição.

O plano de controle não receberá PII nem conteúdo acadêmico. Sua telemetria será limitada por contrato de dados a identificadores técnicos não pessoais, versão, disponibilidade e estado dos componentes. Dados operacionais potencialmente identificáveis, como IPs e mensagens de erro brutas, não atravessarão esse limite sem decisão posterior explícita e controles próprios.

No modelo self-hosted, a integração com o plano de controle será opcional, iniciada de dentro para fora e desativável. O produto deverá continuar funcionando sem conectividade permanente com serviços administrados pelo LICE. Não haverá conta mestre ou acesso oculto de suporte; qualquer suporte com acesso ao plano de dados dependerá de autorização institucional, terá duração e escopo limitados e será auditado.

### Decisões para o MVP

O MVP comprovará portabilidade sem tentar entregar, desde já, a operação de produção dos três modelos:

- aplicação e serviços serão empacotados como imagens OCI;
- PostgreSQL será a persistência principal, com migrações versionadas;
- autenticação usará OIDC, evitando contrato proprietário com um único provedor de identidade;
- arquivos futuros usarão uma abstração compatível com armazenamento de objetos, sem tornar um provedor de nuvem requisito do domínio;
- configuração operacional virá do ambiente e de segredos externos ao artefato;
- o mesmo código suportará execução compartilhada e execução de tenant único;
- o ambiente local e a demonstração com dados sintéticos serão reproduzíveis por Docker Compose;
- dados acadêmicos não serão enviados à telemetria técnica;
- Kubernetes, Helm, OpenTofu/Terraform, instalação *air-gapped* e automação completa de atualização ficam fora da primeira fatia funcional.

Essa preparação não significa que o MVP já seja certificado ou esteja pronto para operar dados reais em produção, nem que toda universidade pública esteja automaticamente autorizada a usar qualquer uma das modalidades.

## Consequências

### Positivas

- Instituições poderão escolher o nível de controle e isolamento compatível com seus riscos e normas.
- O mesmo conjunto de regras acadêmicas, migrações e correções de segurança atenderá aos três modelos.
- A portabilidade reduz dependência de um provedor específico e facilita saída, restauração e migração.
- A separação dos planos limita a exposição de dados acadêmicos ao serviço central.

### Custos e restrições

- A matriz de testes deverá cobrir modo compartilhado, tenant único e diferentes versões suportadas de dependências.
- Self-hosted aumenta o custo de suporte, diagnóstico, atualização e compatibilidade.
- Funcionalidades não poderão depender silenciosamente de serviços externos obrigatórios.
- Mudanças de banco e de configuração precisarão de caminhos seguros de atualização e reversão compatíveis com instalações atrasadas.
- BYOC e self-hosted exigirão documentação operacional, política de versões suportadas, backup, recuperação e responsabilidades contratuais antes de uso real.

## Questões adiadas

- provedores e regiões inicialmente homologados;
- política de suporte e defasagem máxima de versões;
- arquitetura e retenção de backups;
- gestão institucional de chaves criptográficas;
- instalação sem acesso à internet;
- mecanismo de atualização e retorno de versão;
- evidências e certificações exigidas em uma contratação real.
