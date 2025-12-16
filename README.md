# ttl-operator

Kubernetes TTL Operator는 Kubernetes 리소스에 TTL(Time To Live) 기능을 제공하는 Operator

리소스 생성 후 지정된 시간이 지나면 자동으로 삭제

## 기능

- 리소스 생성 시 TTL(초 단위) 설정
- TTL 만료 시 자동 삭제
- 만료 상태 및 시간 추적

## 사전 요구사항

- Kubernetes 클러스터 (v1.20+)
- kubectl 설치 및 클러스터 접근 권한
- Go 1.21+ (개발 시)
- Docker 또는 Podman (이미지 빌드 시)

## 설치

### 1. CRD 설치

```bash
make install
```

### 2. Operator 배포

```bash
# 이미지 빌드 (선택사항)
make docker-build IMG=parkseoyeon/ttl-operator:latest

# Operator 배포
make deploy IMG=parkseoyeon/ttl-operator:latest
```

또는 로컬에서 실행:

```bash
make run
```

## 사용 방법

### TTLResource 생성

TTLResource를 생성하여 리소스에 TTL을 설정할 수 있습니다.

```yaml
apiVersion: ttl.example.com/v1alpha1
kind: TTLResource
metadata:
  name: ttlresource-example
spec:
  ttlSeconds: 60  # 60초 후 자동 삭제
```

### 예제

```bash
# 샘플 리소스 생성
kubectl apply -f config/samples/ttl_v1alpha1_ttlresource.yaml

# 리소스 상태 확인
kubectl get ttlresource ttlresource-test -o yaml

# 리소스 목록 확인
kubectl get ttlresource
```

### TTLResource 필드 설명

#### Spec 필드

- `ttlSeconds` (필수): TTL 시간을 초 단위로 지정합니다. 0으로 설정하면 삭제되지 않습니다.

#### Status 필드

- `expired`: TTL이 만료되었는지 여부 (boolean)
- `createdAt`: 리소스가 생성된 시각
- `expiredAt`: TTL 만료 시각

### 예제 시나리오

#### 30초 후 자동 삭제되는 리소스

```yaml
apiVersion: ttl.example.com/v1alpha1
kind: TTLResource
metadata:
  name: short-lived-resource
spec:
  ttlSeconds: 30
```

#### 1시간 후 자동 삭제되는 리소스

```yaml
apiVersion: ttl.example.com/v1alpha1
kind: TTLResource
metadata:
  name: hourly-resource
spec:
  ttlSeconds: 3600  # 3600초 = 1시간
```

#### TTL 없이 상태만 추적하는 리소스

```yaml
apiVersion: ttl.example.com/v1alpha1
kind: TTLResource
metadata:
  name: no-ttl-resource
spec:
  ttlSeconds: 0  # 0으로 설정하면 삭제되지 않음
```

### 실제 사용 코드


```
apiVersion: v1
kind: Pod
metadata:
  name: test-pod-sy
  annotations:
    ttl.example.com/ttl-seconds: "10"  # 10초 후 자동 삭제
spec:
  containers:
  - name: nginx
    image: nginx:latest
```

```
apiVersion: v1
kind: Service
metadata:
  name: test-service-sy
  annotations:
    ttl.example.com/ttl-seconds: "30"  # 30초 후 자동 삭제
spec:
  selector:
    app: nginx
  ports:
  - port: 80
    targetPort: 80
```

## 핵심 파일 설명

이 프로젝트의 주요 파일들과 역할을 설명합니다.

### 1. `api/v1alpha1/ttlresource_types.go`

**역할**: Custom Resource의 타입 정의

이 파일은 TTLResource CRD의 Go 타입 정의를 포함합니다:

- **TTLResourceSpec**: 사용자가 설정하는 원하는 상태
  - `ttlSeconds`: TTL 시간(초 단위)
  
- **TTLResourceStatus**: Operator가 관리하는 관찰된 상태
  - `expired`: 만료 여부
  - `createdAt`: 리소스 생성 시각
  - `expiredAt`: TTL 만료 시각


### 2. `internal/controller/ttlresource_controller.go`

**역할**: TTLResource를 관리하는 컨트롤러 로직

이 파일은 Operator의 핵심 비즈니스 로직을 구현합니다:



### 3. `cmd/main.go`

**역할**: Operator의 진입점 및 초기화

이 파일은 Operator의 메인 함수를 포함하며 다음을 수행합니다:

- Kubernetes Scheme 설정 (CRD 타입 등록)
- Controller Manager 생성 및 설정
  - 메트릭스 서버 설정
  - 헬스체크 엔드포인트 설정
  - 리더 선출(Leader Election) 설정
- TTLResourceReconciler 등록
- Manager 시작

### 4. `config/crd/bases/ttl.example.com_ttlresources.yaml`

**역할**: CustomResourceDefinition 매니페스트

이 파일은 Kubernetes에 TTLResource CRD를 정의하는 YAML입니다:

- CRD 메타데이터 (그룹, 버전, 이름)
- OpenAPI 스키마 정의
  - Spec 필드: `ttlSeconds` (필수, integer)
  - Status 필드: `expired`, `createdAt`, `expiredAt`
- Status 서브리소스 활성화

이 파일은 `make manifests` 명령으로 자동 생성됩니다.

### 5. `config/manager/manager.yaml`

**역할**: Operator 배포 매니페스트

이 파일은 Operator를 Kubernetes에 배포하기 위한 Deployment 정의입니다:

- Namespace: `system`
- Deployment: `controller-manager`
  - 컨테이너 이미지: `controller:latest`
  - 리더 선출 활성화 (`--leader-elect`)
  - 헬스체크 엔드포인트 (`/healthz`, `/readyz`)
  - 리소스 제한 설정
  - 보안 컨텍스트 설정 (Pod Security Standards 준수)

### 6. `Makefile`

**역할**: 프로젝트 빌드 및 배포 자동화

주요 타겟:

- `make install`: CRD 설치
- `make deploy`: Operator 배포
- `make build`: 바이너리 빌드
- `make docker-build`: Docker 이미지 빌드
- `make test`: 단위 테스트 실행
- `make manifests`: CRD 및 RBAC 매니페스트 생성
- `make generate`: DeepCopy 코드 생성

## 개발

### 로컬 개발 환경 설정

```bash
# 의존성 설치
go mod download

# 코드 생성
make generate

# 매니페스트 생성
make manifests

# 빌드
make build

# 테스트 실행
make test
```

### 코드 포맷팅 및 린트

```bash
# 코드 포맷팅
make fmt

# 린트 검사
make lint

# 린트 자동 수정
make lint-fix
```

## 제거

### Operator 제거

```bash
make undeploy
```

### CRD 제거

```bash
make uninstall
```

## 라이선스

Apache License 2.0
