# HANDOFF — driver `blob/huaweiblob` (Huawei Cloud OBS)

> Artefato de continuidade entre assistentes. Gerado em 2026-11-22 21:20
> (America/Sao_Paulo) a partir de inspeção direta do working tree.
> **ATUALIZAÇÃO 2026-11-22 21:30 (decisão do founder):** entregar o **estado
> atual** via branch + PR, **incluindo este handoff**, sem concluir os 79
> `.replay`. As recomendações do §3 continuam válidas como lista do que falta;
> foram intencionalmente adiadas. Remover este arquivo antes do merge, se desejar.

## 0. TL;DR

O driver está **escrito, compila e passa nos testes unitários**, mas **não está
entregue**: `TestConformance` falha porque os **79 golden files `.replay` não
existem**. O diretório `blob/huaweiblob/` é **100% untracked**, não há branch de
feature, não há commit, não há PR e os gates locais estão sem marker.

O próximo assistente precisa: (1) formatar, (2) gravar os replays contra um mock
OBS local, (3) validar replay offline, (4) registrar o pacote no site/docs do
Go CDK, (5) rodar os gates e (6) commitar/push/PR.

**Atenção:** o repositório usa `master`, não `main`. Tanto `google/go-cloud`
quanto `marcvlima/go-cloud` têm `master` como default branch.

---

## 1. Estado real verificado

### Git

| Item | Valor |
| --- | --- |
| Repo | `/tmp/go-cloud-fork` |
| Branch atual | `master` |
| HEAD | `313306621a17532f2f459522eeccfe84752f402e` |
| Commit HEAD | `2026-11-15 11:49:25 -0800 blob: add some test coverage (#3847)` |
| `git status --short` | `?? blob/huaweiblob/` (nada tracked alterado) |
| `git diff --stat` | vazio |
| `origin` | `https://github.com/marcvlima/go-cloud.git` |
| `upstream` | `https://github.com/google/go-cloud.git` |
| `origin/master...HEAD` | `0 0` (sem ahead/behind) |
| Branches | só `master`, `origin/master`, `origin/HEAD -> origin/master` |
| Default branch upstream | `master` |
| PRs abertos | nenhum (nem no fork, nem de `marcvlima` no upstream) |
| `gh auth status` | logado como `marcvlima`, escopos `gist, read:org, read:packages, repo, workflow` |

### Go / dependências

- `go version go1.26.7 linux/amd64`.
- `go.mod` e `go.sum` **inalterados**; `go mod tidy` não produz diff.
- `github.com/google/go-replayers/httpreplay v1.2.0` já é dependência do módulo
  raiz (`go.mod:58`) — **não é necessário adicionar dependência**.
- `blob/huaweiblob` pertence ao módulo raiz (`.`), então **não** deve entrar em
  `allmodules` (esse arquivo lista módulos com `go.mod` próprio, não pacotes).

### Arquivos da feature (todos untracked)

| Arquivo | Linhas | Conteúdo |
| --- | --- | --- |
| `blob/huaweiblob/huaweiblob.go` | 1376 | implementação do driver |
| `blob/huaweiblob/huaweiblob_test.go` | 409 | harness + testes unitários + `TestConformance` |
| `blob/huaweiblob/example_test.go` | 78 | 2 exemplos (`ExampleOpenBucket`, `Example_openBucketFromURL`) |
| `blob/huaweiblob/testdata/` | — | **diretório vazio**, nenhum `.replay` |

### Resultado dos comandos

| Comando | Resultado |
| --- | --- |
| `go build ./blob/huaweiblob/...` | **OK** (exit 0) |
| `go vet ./blob/huaweiblob/...` | **OK** (exit 0) |
| `go test ./blob/huaweiblob/... -run 'TestObsErrorMessage\|TestWithKey\|TestOpenBucket\|TestOpenBucketFromURL\|Example' -v` | **PASS** |
| `go test ./blob/huaweiblob/... -count=1` | **FAIL** — `TestConformance` |
| `gofmt -s -l blob/huaweiblob` | **3 arquivos sujos** (ver §3.A) |
| `bash internal/website/listnewpkgs.sh` | retorna `blob/huaweiblob` (falta `_index.md`) |
| `quality-guard status` | `valid: NO — no quality-gate marker — run: quality-guard run` |
| `plan-guard status` | `active plan: (none)` / `valid: NO — no plan-gate marker` |
| `.risegen/` | não existe |
| `quality-surfaces.yaml` | **não existe neste fork** — conferir antes de confiar no `quality-guard run` |

`quality-guard status` fingerprint:
`sha256:b225635a0e1eb1607e268ce2a706a119c1bf5e34576b5cc84c4ff482286b98d5`.
`plan-guard status` fingerprint:
`sha256:f6c5d9876314d1c3f80523f8a8255dc81c4e647b822b496862f8f4f1605582cc`.

### Falha exata de `TestConformance`

`setup.NewRecordReplayClient` (`internal/testing/setup/setup.go:73-108`) usa
`filepath.Join("testdata", t.Name()+".replay")`. Em replay mode, sem o arquivo,
cada subtest morre com:

```
drivertest.go:1187: Replaying from golden file testdata/TestConformance/....replay
drivertest.go:1187: open testdata/TestConformance/....replay: no such file or directory
```

- **19 grupos de topo** falham: `TestNonexistentBucket`, `TestList`,
  `TestListWeirdKeys`, `TestListDelimiters`,
  `TestDirsWithCharactersBeforeDelimiter`, `TestRead`, `TestAttributes`,
  `TestWrite`, `TestCanceledWrite`, `TestConcurrentWriteAndRead`,
  `TestUploadDownload`, `TestMetadata`, `TestMD5`, `TestCopy`, `TestDelete`,
  `TestKeys`, `TestSignedURL`, `TestIfNotExist`, `TestAs`.
- **79 paths `.replay` distintos** ausentes (contagem confirmada por
  `grep 'Replaying from golden' | sort -u | wc -l`).
- Reproduzir a lista:
  `go test ./blob/huaweiblob/... -count=1 2>&1 | grep 'Replaying from golden' | sed 's/.*golden file //; s/\.replay:.*$//' | sort -u`
- Referência de volume: `blob/s3blob/testdata` tem 164 `.replay`.

---

## 2. O que JÁ está implementado (não reescrever)

`blob/huaweiblob/huaweiblob.go`:

- Registro `init()` em `blob.DefaultURLMux().RegisterBucket(Scheme, &URLOpener{})`,
  `Scheme = "obs"`, `defaultRegion = "cn-north-1"`, `defaultPageSize = 1000`.
- `Options{Endpoint, BucketName, AccessKeyID, SecretKey, Region, HTTPClient}` com
  validação dos 4 campos obrigatórios; `URLOpener.OpenBucketURL` aceita
  `endpoint`, `access_key`, `secret_key`, `region` e rejeita parâmetros
  desconhecidos/duplicados.
- `OpenBucket` / `openBucket` retornando `driver.Bucket`.
- Estilos de endpoint: **virtual-hosted + HTTPS** quando `Endpoint` não começa
  com `http` (`https://<bucket>.<endpoint>/<path>`); **path-style** quando começa
  com `http` (`<endpoint>/<bucket>/<path>`). Ver `newRequest` em `:874-899`.
- Assinatura SigV4-compatible com service name `obs`
  (`generateSignatureV4` `:957-995`, `canonicalQuery`, `canonicalURI`,
  `rfc3986Encode`, `hmacSHA256`), headers `X-Obs-Date`,
  `X-Obs-Content-Sha256`, `Authorization`.
- Operações sobre a REST API S3-compatible:
  - `Attributes` (`HEAD /<key>`) — `ETag`, `Content-Length`, `Last-Modified`,
    `Content-Type`, metadados `x-obs-meta-*` / `x-amz-meta-*`.
  - `ListPaged` (`GET /?list-type=2`) — `listBucketResult`, `listContents`,
    `listCommonPrefix`, continuation token, delimitadores.
  - `NewRangeReader` (`GET /<key>` + `Range`) — `reader`, `Content-Range`,
    `getSize`, `parseHTTPTime`.
  - `NewTypedWriter` / `writeData` / `putObject` / `Upload` (`PUT /<key>`) —
    `Content-MD5`, detecção de Content-Type, `If-None-Match` / condicionais.
  - `Copy` (`PUT /<dstKey>` + `x-copy-source`).
  - `Delete` (`DELETE /<key>`), com `HEAD` prévio porque OBS devolve sucesso
    mesmo para chave inexistente.
  - `SignedURL` retorna `errNotImplemented` → mapeia para
    `gcerrors.Unimplemented` (comentário em `:843-848`).
- `obsError{StatusCode, Key, Code, Message}` + `xmlErr` + `ErrorCode` mapeado
  para `NotFound`, `PermissionDenied`, `FailedPrecondition`, `Unimplemented`,
  `Unknown`. **`withKey()` (Fase A) já concluída** e aplicada em
  `Attributes`/`Reader`/`Write`/`Copy`/`Delete`; preserva a identidade do
  `*obsError` para `errors.As` e `blob.Bucket.ErrorAs`.
- Hooks `As`: Bucket `*http.Client`, Error `*obsError`, Reader
  `*http.Response`, Attributes `*http.Header`, ListObject `listContents` /
  `listCommonPrefix`, callbacks `Before*` → `**http.Request`.
- Escaping via `gocloud.dev/internal/escape` (`escapeKey`, `unescapeKey`,
  `unescapeMetadataKey`, `unescapeMetadataValue`, `eTagToMD5`).
- **Extras sem cobertura de teste:** `VersioningConfiguration`,
  `LifecycleConfiguration`, `LifecycleRule` e subtipos (`:1176-1250`), mais
  `EnableBucketVersioning`, `GetBucketVersioning`,
  `PutLifecycleConfiguration`, `GetLifecycleConfiguration` (`:1252-1376`).
  São ~200 linhas de API pública fora do contrato `driver.Bucket`. **Decisão em
  aberto:** manter (e testar) ou remover antes do PR para não subir superfície
  não testada.

`blob/huaweiblob/huaweiblob_test.go`:

- `harness` com `setup.NewRecordReplayClient`, filtros do recorder:
  `RemoveRequestHeaders("Authorization")`, `ClearHeaders("X-Obs-Date")`,
  `ClearQueryParams("X-Obs-Date")`, `ClearHeaders("User-Agent")`.
- Constantes de record: `bucketName = "go-cloud-testing-2"`,
  `region = "cn-north-4"`,
  `endpoint = "obs.cn-north-4.myhuaweicloud.com"` (**virtual-hosted**).
- `MakeDriverForNonexistentBucket` usa `"go-cdk-bucket-does-not-exist"`.
- `SupportsCopy() = true`, `SupportsNonSlashDelimiters() = true`,
  `SkipDownloadAt() = true` (mesma escolha do `s3blob`, ver
  `blob/s3blob/s3blob_test.go:89-90`).
- `obsAccessKeyID` / `obsSecretKey`: em `--record` exigem
  `OBS_ACCESS_KEY_ID` / `OBS_SECRET_KEY`; em replay devolvem
  `FAKE_ACCESS_KEY` / `FAKE_SECRET_KEY`.
- `verifyObsAs` implementa todos os checks de `As`; `Name()` contém `/`
  (`"verify OBS requests/responses can be accessed through As"`), o que gera
  um golden com diretório extra:
  `testdata/TestConformance/TestAs/verify_OBS_requests/responses_can_be_accessed_through_As.replay`.
  É esperado, mas simplificar o `Name()` reduz a chance de confusão.
- Testes unitários que já passam: `TestObsErrorMessage` (4 casos),
  `TestWithKey`, `TestOpenBucket` (5 casos), `TestOpenBucketFromURL` (8 URLs).

---

## 3. O que FALTA — ordem de execução

### A. `gofmt -s` (bloqueia o presubmit do upstream)

`internal/testing/runchecks.sh:145-148` roda `gofmt -s -d .` e falha o build.
Hoje `gofmt -s -l blob/huaweiblob` retorna os 3 arquivos `.go`. Diferenças
observadas:

- newline ausente no EOF de `huaweiblob.go`, `huaweiblob_test.go`,
  `example_test.go`;
- alinhamento dos campos de `listBucketResult` em `huaweiblob.go:504-510`.

```bash
cd /tmp/go-cloud-fork
gofmt -s -w blob/huaweiblob
gofmt -s -l blob/huaweiblob   # deve sair vazio
```

Também revisar: `huaweiblob.go` usa cabeçalho `Copyright 2018-2019` (copiado de
outro driver) enquanto os testes usam `2026`.

### B. Mock HTTP OBS local (não existe ainda)

Não há mock, script de geração de replay ou `docker-compose` no repositório.
É preciso construir um servidor HTTP local (Go `httptest` num processo
separado, ou binário auxiliar **fora do tree final**) que responda à API usada
pelo driver, com estado em memória por bucket:

- `PUT /<bucket>/<key>` — grava corpo, `Content-Type`, `Content-MD5`,
  metadados `x-obs-meta-*`; devolve `ETag` = MD5 hex entre aspas; honrar
  `If-None-Match: *` / condicionais de `TestIfNotExist`.
- `HEAD /<bucket>/<key>` — `404` para chave/bucket inexistente; senão
  `Content-Length`, `ETag`, `Last-Modified`, `Content-Type`, metadados.
- `GET /<bucket>/<key>` — suporte a `Range` com `206` + `Content-Range`
  (incluindo leitura no meio, `length = -1`, offsets), `416` quando aplicável.
- `GET /<bucket>/?list-type=2` — XML `ListBucketResult` com `Contents`,
  `CommonPrefixes`, `IsTruncated`, `NextContinuationToken`; respeitar
  `prefix`, `delimiter`, `max-keys`, `continuation-token`; chaves com
  caracteres especiais (unicode, `//`, `..`, `\\`, espaços, aspas,
  non-UTF8) para `TestKeys` / `TestListWeirdKeys` / `TestListDelimiters`.
- `PUT /<bucket>/<dstKey>` com `x-copy-source` — copia corpo + atributos
  (`TestCopy/DoesNotAliasAttributes` exige que não haja aliasing).
- `DELETE /<bucket>/<key>` — remove; o driver faz `HEAD` antes.
- Erros em XML (`<Error><Code>…</Code><Message>…</Message></Error>`) com
  status 400/403/404/412 para exercitar `obsError` / `ErrorCode` / `ErrorAs`.
- `TestNonexistentBucket`: qualquer request para
  `go-cdk-bucket-does-not-exist` deve devolver 404.
- `TestConcurrentWriteAndRead` e `TestUploadDownload`: o mock precisa ser
  concorrente-safe.
- `TestSignedURL`: nenhuma chamada HTTP é necessária além da criação do
  harness; golden vazio é aceitável se o recorder fechar sem requests.
- `Before*` / `As`: o recorder intercepta `*http.Request`/`*http.Response`
  — não precisa de lógica extra no mock.

**Porta fixa** é importante: as URLs gravadas no `.replay` precisam ser iguais
em record e em replay.

### C. Ajustar o harness para o mock

`endpoint` hoje é virtual-hosted HTTPS (`obs.cn-north-4.myhuaweicloud.com`) —
**incompatível com `127.0.0.1`**. Mudar para path-style, por exemplo
`http://127.0.0.1:PORT` (constante ou env var lida no teste), em
`MakeDriver` e `MakeDriverForNonexistentBucket`. O driver já trata prefixo
`http` como path-style (`huaweiblob.go:888-898`).

Em `--record`, `obsAccessKeyID`/`obsSecretKey` exigem as env vars — contra o
mock podem ser valores falsos, já que a assinatura não é validada e o header
`Authorization` é removido pelo recorder:

```bash
OBS_ACCESS_KEY_ID=FAKE OBS_SECRET_KEY=FAKE \
  go test ./blob/huaweiblob/... -run TestConformance -record -count=1 -v
```

Alternativa (se não quiser mock): gravar contra OBS real com credenciais
reais e o bucket `go-cloud-testing-2` em `cn-north-4`. **Não é o caminho
preferido** — depende de rede externa e de conta Huawei; o objetivo declarado
da feature é rodar offline.

### D. Gravar os 79 goldens

`setup.NewRecordReplayClient` cria `testdata/<t.Name()>.replay` e faz
`MkdirAll(filepath.Dir(path))` — os diretórios aninhados por subtest são
criados automaticamente pelo `-record`.

```bash
cd /tmp/go-cloud-fork
go test ./blob/huaweiblob/... -run TestConformance -record -count=1 -v
find blob/huaweiblob/testdata -name '*.replay' | wc -l   # esperado: 79
```

Conferir que **nenhum segredo** real foi para os goldens: `Authorization` é
removido, `X-Obs-Date`/`User-Agent` são clearados, mas revisar `x-obs-meta-*`,
`x-copy-source` e corpos antes de commitar.

### E. Validar replay offline (aceite binário)

```bash
cd /tmp/go-cloud-fork
go test ./blob/huaweiblob/... -count=1        # deve ser PASS, sem -record
go test ./blob/huaweiblob/... -count=1 -v | grep -c '^--- PASS: TestConformance'
go build ./blob/huaweiblob/... && go vet ./blob/huaweiblob/...
```

Critério: **todos os 19 grupos de `TestConformance` verdes em replay mode, sem
rede e sem `OBS_*` no ambiente.**

### F. Registro do pacote no site/docs do Go CDK (presubmit)

`internal/testing/runchecks.sh` falha se isso não for feito:

1. **Hugo package meta** — `internal/website/listnewpkgs.sh` hoje imprime
   `blob/huaweiblob`. Criar
   `internal/website/content/blob/huaweiblob/_index.md` espelhando
   `internal/website/content/blob/s3blob/_index.md`:
   ```
   ---
   title: gocloud.dev/blob/huaweiblob
   type: pkg
   ---
   ```
   (Alternativa: rodar `internal/website/makeimports.sh`, se existir.)
2. **Examples** — os 2 exemplos de `example_test.go` entram no
   `gatherexamples`. Regenerar:
   ```bash
   internal/website/gatherexamples/run.sh > internal/website/data/examples.json
   ```
   O check é `runchecks.sh:181-184`.
3. **How-to** — acrescentar seção "Huawei Cloud OBS" em
   `internal/website/content/howto/blob/_index.md` (seção
   `## Supported Storage Services`) com `{{< goexample ... >}}` para os dois
   exemplos, seguindo o padrão de `### S3 {#s3}` (linhas 215-265).
4. **Blank import** — considerar `_ "gocloud.dev/blob/huaweiblob"` em
   `blob/example_test.go` (hoje só `fileblob`, `gcsblob`, `s3blob` nas linhas
   29-32) para que o scheme `obs` fique disponível nos exemplos de
   `blob.OpenBucket`.
5. **Dependências** — `internal/testing/listdeps.sh > internal/testing/alldeps`
   só se houver mudança real de `go.mod`. Como `httpreplay v1.2.0` já está em
   `go.mod:58` e `go mod tidy` não gera diff, **não deve** ser necessário.
6. **Presubmit completo**: `internal/testing/runchecks.sh` (inclui
   `go test ./...` por módulo, `go mod tidy`, `wire diff`, `gofmt -s`,
   `gatherexamples`, `listnewpkgs`).

### G. Gates do holding

- `quality-guard run` antes do commit (`quality-guard status` hoje é `NO`), com
  teste pareado para cada mudança de código de produção. **Atenção:** não há
  `quality-surfaces.yaml` nem `.risegen/` neste fork — verificar como o gate
  está configurado aqui antes de assumir que `run` cobre o pacote.
- `plan-guard`: nenhum plano ativo (`active plan: (none)`). Se o próximo
  assistente registrar um plano, rodar `plan-guard register|run|status`.
- Trailer `ABG: <repo>#<issue>` no commit, conforme a skill
  `quality-enforcement` / `AGENTS.md`. **Não há issue aberta** no fork nem no
  upstream — criar uma ou alinhar com o founder antes de inventar o número.
- Não usar `--no-verify`.

### H. Git / entrega final

O working tree sozinho não é entrega. Sequência:

```bash
cd /tmp/go-cloud-fork
git status --short                 # reconfirmar
git branch --show-current          # master
git rev-parse HEAD                 # reconfirmar antes de commitar
git switch -c feat/huaweiblob-obs-driver
git add HANDOFF.md blob/huaweiblob
# Entrega de 21:30 = estado atual. Após concluir o §3, adicionar também
# _index.md, examples.json, howto/blob/_index.md e blob/example_test.go.
git commit -m "blob: add Huawei Cloud OBS driver"   # + trailer ABG
git push -u origin feat/huaweiblob-obs-driver
gh pr create --repo google/go-cloud --base master --head marcvlima:feat/huaweiblob-obs-driver
```

- **Nunca push direto em `master`** (regra do workspace; `master` é o default
  do upstream).
- Se houver outra sessão de agente no mesmo worktree, **provisionar worktree
  separado** (`git worktree add`) antes de continuar — isolamento é regra
  (`AGENTS.md`), e `HEAD` não deve ser assumido estável.
- Verificar o PR no CI do upstream antes de declarar concluído.

---

## 4. Armadilhas / decisões em aberto

1. **`main` não existe.** O usuário disse "subir para a main"; o alvo real é
   `master` nos dois remotes.
2. **`endpoint` do harness é virtual-hosted HTTPS.** Qualquer mock local
   exige a troca para path-style `http://127.0.0.1:PORT` (§3.C).
3. **Goldens não podem ser copiados de `s3blob`.** Headers (`X-Obs-Date`,
   `x-obs-meta-*`), corpo XML e esquema de assinatura diferem.
4. **`verifyObsAs.Name()` contém `/`**, gerando um nível extra de diretório no
   golden. Se o recorder tiver problema com isso, simplificar o nome.
5. **`SkipDownloadAt() = true`** já alinhado ao `s3blob`; não remover sem
   justificativa.
6. **`SignedURL` deliberadamente `Unimplemented`** — `TestSignedURL` deve
   passar pulando as asserções, não implementando presigned URL.
7. **Tipos de Versioning/Lifecycle** (~200 linhas, `:1176-1376`) não têm
   testes pareados e não fazem parte de `driver.Bucket`. Decidir manter com
   testes ou remover antes do PR.
8. **`example_test.go`** usa `Endpoint: "https://obs.cn-north-4.myhuaweicloud.com"`
   (path-style, porque começa com `http`) e a URL de exemplo usa
   `endpoint=https%3A%2F%2F...`. Consistente com a doc de `Options`, mas
   diferente da constante do harness (sem prefixo). Vale uniformizar a doc.
9. **Doc do pacote** (`huaweiblob.go:49-51`) contém uma frase truncada/
   sem sentido: *"To read the escaped key back, use the `OBS` as the opaque
   API? No: decoders are internal..."*. Limpar antes do PR.
10. **Credenciais em `--record`**: se optar por OBS real, `OBS_ACCESS_KEY_ID` /
    `OBS_SECRET_KEY` são obrigatórias e o recorder **remove** `Authorization` —
    ainda assim, revisar os goldens por segredo antes do commit.

---

## 5. Checklist de aceite final

- [ ] `gofmt -s -l blob/huaweiblob` vazio.
- [ ] Mock OBS local implementado (ou gravação contra OBS real decidida).
- [ ] Harness em path-style com porta fixa.
- [ ] 79 `.replay` gravados em `blob/huaweiblob/testdata/`, sem segredos.
- [ ] `go test ./blob/huaweiblob/... -count=1` **PASS offline** (19 grupos).
- [ ] `go build` + `go vet` verdes.
- [ ] `internal/website/content/blob/huaweiblob/_index.md` criado;
      `listnewpkgs.sh` sem saída.
- [ ] `examples.json` regenerado; `gatherexamples` diff limpo.
- [ ] Seção how-to + blank import revisados.
- [ ] `internal/testing/runchecks.sh` verde.
- [ ] `quality-guard run` verde com marker válido; trailer `ABG` no commit.
- [ ] Branch `feat/*` a partir de `master`, commit, push para `origin`, PR
      para `google/go-cloud:master`, CI verde.
- [ ] Definir se `HANDOFF.md` permanece no PR (incluído na entrega de 21:30).
