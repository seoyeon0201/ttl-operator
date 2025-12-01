# ttl-operator

Kubernetes TTL Operator는 Kubernetes 리소스에 TTL(Time To Live) 기능을 제공하는 Operator입니다. 
리소스 생성 후 지정된 시간이 지나면 자동으로 삭제됩니다.

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
make docker-build IMG=your-registry/ttl-operator:latest

# Operator 배포
make deploy IMG=your-registry/ttl-operator:latest
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
