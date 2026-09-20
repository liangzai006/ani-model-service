package kubernetes

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestBuildImportJob(t *testing.T) {
	backoff := int32(1)
	spec := ImportJobSpec{
		Namespace:             "ani-model",
		Name:                  "import-task-1",
		Image:                 "registry.example/model-importer:latest",
		Command:               []string{"/bin/import"},
		Args:                  []string{"--task", "task-1"},
		Env:                   []corev1.EnvVar{{Name: "SOURCE", Value: "modelscope"}},
		Volumes:               []corev1.Volume{{Name: "staging", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
		VolumeMounts:          []corev1.VolumeMount{{Name: "staging", MountPath: "/staging"}},
		ServiceAccountName:    "model-importer",
		Labels:                map[string]string{"ani.example/task-id": "task-1"},
		Annotations:           map[string]string{"ani.example/attempt": "1"},
		BackoffLimit:          &backoff,
		ActiveDeadlineSeconds: func() *int64 { v := int64(3600); return &v }(),
	}

	job, err := BuildImportJob(spec)
	if err != nil {
		t.Fatalf("BuildImportJob() error = %v", err)
	}
	if job.Namespace != spec.Namespace || job.Name != spec.Name {
		t.Fatalf("job identity = %s/%s, want %s/%s", job.Namespace, job.Name, spec.Namespace, spec.Name)
	}
	if job.Labels[ManagedLabel] != "true" || job.Labels["ani.example/task-id"] != "task-1" {
		t.Fatalf("job labels = %#v", job.Labels)
	}
	if got := job.Spec.Template.Spec.RestartPolicy; got != corev1.RestartPolicyNever {
		t.Fatalf("restart policy = %q, want Never", got)
	}
	container := job.Spec.Template.Spec.Containers[0]
	if container.Name != ImportContainerName || container.Image != spec.Image {
		t.Fatalf("container = %#v", container)
	}
	if len(container.Command) != 1 || container.Command[0] != "/bin/import" {
		t.Fatalf("command = %#v", container.Command)
	}
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 1 {
		t.Fatalf("backoff limit = %#v", job.Spec.BackoffLimit)
	}
}

func TestImportJobClientCreateAndStatus(t *testing.T) {
	client := NewImportJobClient(k8sfake.NewSimpleClientset())
	job, err := client.Create(context.Background(), ImportJobSpec{
		Namespace: "ani-model",
		Name:      "import-task-1",
		Image:     "registry.example/model-importer:latest",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if job.Name != "import-task-1" {
		t.Fatalf("created job name = %q", job.Name)
	}

	job.Status.Active = 1
	if _, err := client.client.BatchV1().Jobs(job.Namespace).UpdateStatus(context.Background(), job, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}
	status, err := client.Status(context.Background(), job.Namespace, job.Name)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Phase != JobRunning || status.Active != 1 {
		t.Fatalf("status = %#v", status)
	}

	job.Status.Active = 0
	job.Status.Succeeded = 1
	job.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue, Reason: "Completed"}}
	if _, err := client.client.BatchV1().Jobs(job.Namespace).UpdateStatus(context.Background(), job, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}
	status, err = client.Status(context.Background(), job.Namespace, job.Name)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Phase != JobSucceeded || status.Reason != "Completed" {
		t.Fatalf("completed status = %#v", status)
	}
}

func TestBuildImportJobRequiresIdentityAndImage(t *testing.T) {
	for name, spec := range map[string]ImportJobSpec{
		"namespace": {Name: "import-task-1", Image: "image"},
		"name":      {Namespace: "ani-model", Image: "image"},
		"image":     {Namespace: "ani-model", Name: "import-task-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildImportJob(spec); err == nil {
				t.Fatal("BuildImportJob() error = nil")
			}
		})
	}
}
