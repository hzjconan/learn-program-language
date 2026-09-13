{{/*
_helpers.tpl：命名模板（D19 §6.4）。

⭐ 下划线开头的文件 helm 不会当成资源渲染，只用来 define。
每个 define 的名字要带 chart 前缀 —— define 是【全局】的，
subchart 里同名会互相覆盖。
*/}}

{{/*
release 名 + chart 名，截到 63 字符（K8s 名字上限），去掉尾部的 -。
release 名已经包含 chart 名时不重复（helm install golearn-api ./golearn-api → golearn-api，
而不是 golearn-api-golearn-api）。
*/}}
{{- define "golearn-api.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/*
公共 labels。⭐ 这段被 include 之后要接 nindent，所以这里【不缩进】——
缩进由调用方决定（§6.4 为什么用 include 不用 template）。
*/}}
{{- define "golearn-api.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{/*
selector labels —— Deployment.spec.selector 和 Pod labels 必须【完全一致】，
而且 selector 一旦创建就不可变，所以这里只放不会变的两个。
⚠️ 别把 version 放进 selector：升级 appVersion 时 selector 变了，Deployment 会拒绝更新。
*/}}
{{- define "golearn-api.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
镜像：tag 没给就用 appVersion。
*/}}
{{- define "golearn-api.image" -}}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) -}}
{{- end -}}
