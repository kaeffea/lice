# MVP demonstrável

## Objetivo

Entregar até **29 de julho de 2026** uma demonstração executável do fluxo de matrícula regular, do provisionamento de uma instituição à publicação do resultado. O MVP deve validar o núcleo do produto com profundidade suficiente para receber feedback, não simular um ERP universitário completo.

Todos os dados da demonstração serão **sintéticos**. O MVP não será apresentado como pronto para operar dados reais ou substituir um sistema institucional em produção.

## Personas ativas

- **Operador da plataforma:** cria, ativa ou suspende uma instituição e cadastra seu administrador inicial.
- **Administrador do tenant/registro acadêmico:** configura dados básicos da instituição, estrutura acadêmica, período e calendário.
- **Coordenação/secretaria:** configura oferta, vagas e reservas e acompanha solicitações.
- **Estudante:** consulta informações acadêmicas necessárias, solicita matrícula e recebe o resultado.

Docentes e outros vínculos poderão existir no modelo quando necessários à oferta, mas não terão uma jornada operacional completa neste recorte.

Administração do tenant e registro acadêmico permanecerão capacidades separadas. Uma mesma conta poderá acumulá-las na demonstração sem transformar essa combinação em uma permissão única.

## Incluído

- autenticação e autorização contextual das personas ativas;
- criação e isolamento de tenants;
- dados básicos e preferências essenciais da instituição;
- estrutura organizacional e acadêmica mínima;
- pessoas com múltiplos vínculos e atribuições com escopo e vigência;
- período letivo e janelas de matrícula versionados;
- curso, matriz curricular, componentes, turmas, vagas e reservas mínimas;
- fatos acadêmicos sintéticos necessários à elegibilidade e classificação;
- solicitação e cancelamento de solicitação dentro da janela permitida;
- visualização provisória da situação da solicitação sem exposição indevida de dados pessoais;
- processamento determinístico da matrícula, sem ultrapassar vagas;
- publicação atômica do resultado;
- auditoria funcional pesquisável e visível na interface conforme autorização;
- execução local reproduzível, testes automatizados das regras críticas e documentação mínima;
- formato canônico mínimo para carregar os dados sintéticos e preparar a futura migração.

## Não incluído

- operação com dados reais ou homologação para uso institucional;
- rematrícula, matrícula extraordinária e turmas de férias;
- lançamento de notas, frequência, diário e consolidação de turma;
- histórico escolar documental completo, diplomas ou assinatura de documentos;
- módulos completos de RH, pesquisa, extensão, biblioteca, patrimônio ou finanças;
- migração integral de sistemas legados;
- cobrança comercial, alta disponibilidade ou suporte operacional de produção;
- Kubernetes, múltiplas nuvens e todas as modalidades de hospedagem concluídas;
- editor genérico de regras, papéis ou estados acadêmicos arbitrários.

## Critérios de sucesso

O recorte estará demonstrável quando:

1. cada persona ativa conseguir concluir sua jornada autorizada pela interface;
2. duas instituições não conseguirem consultar nem alterar os dados uma da outra;
3. uma pessoa puder possuir mais de um vínculo sem duplicação de identidade;
4. o mesmo conjunto de entradas e a mesma versão de política produzirem o mesmo resultado;
5. nenhuma execução alocar mais estudantes que as vagas disponíveis;
6. o resultado não ficar parcialmente publicado em caso de falha;
7. ações administrativas e acadêmicas relevantes aparecerem na auditoria para o escopo autorizado;
8. dados sensíveis, segredos e credenciais não forem expostos na auditoria;
9. os testes críticos passarem em ambiente limpo e a aplicação puder ser iniciada seguindo a documentação;
10. o responsável pelo produto validar cada jornada completa antes do encerramento do MVP.

## Estado atual

Este arquivo define o compromisso e os critérios do MVP. Itens desta lista só poderão ser marcados como concluídos após implementação, testes e validação; sua presença aqui não é evidência de entrega.
