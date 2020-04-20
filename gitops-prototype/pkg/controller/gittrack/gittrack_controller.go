package gittrack

import (
	"context"
	"fmt"
	"strings"
	"time"

	mayadatav1alpha1 "github.com/storage-provisiong-poc/gittrack/pkg/apis/mayadata.io/v1alpha1"
	git "gopkg.in/src-d/go-git.v4"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_gittrack")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new GitTrack Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileGitTrack{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("gittrack-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource GitTrack
	err = c.Watch(&source.Kind{Type: &mayadatav1alpha1.GitTrack{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner GitTrack
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &mayadatav1alpha1.GitTrack{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileGitTrack{}

// ReconcileGitTrack reconciles a GitTrack object
type ReconcileGitTrack struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a GitTrack object and makes changes based on the state read
// and what is in the GitTrack.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileGitTrack) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling GitTrack")

	reconcileGitTrack := reconcile.Result{
		Requeue:      true,
		RequeueAfter: time.Duration(5 * time.Minute),
	}

	// Fetch the GitTrack instance
	instance := &mayadatav1alpha1.GitTrack{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcileGitTrack, nil
		}
		// Error reading the object - requeue the request.
		return reconcileGitTrack, err
	}

	secret, err := r.GetSecret(instance.Spec.DeployKey.SecretName, instance.Spec.DeployKey.SecretNamespace)
	if err != nil {
		log.Info("error in getting secret ", err)
	}

	gitOperations := &GitOperations{
		RepositoryName: instance.Spec.Repository,
		Repository:     &git.Repository{},
		Branch:         instance.Spec.Branch,
		SubPath:        instance.Spec.SubPath,
		Username:       string(secret.Data["username"]),
		Password:       string(secret.Data["password"]),
		Type:           string(instance.Spec.DeployKey.Type),
	}

	fmt.Println("Git operations ", gitOperations)

	// Clone the repo
	clonePath := strings.SplitAfter(gitOperations.RepositoryName, "https://")
	err = gitOperations.clone(clonePath[1])
	if err != nil {
		fmt.Println("error in cloning repo", err)
	}

	// Checkout the desired branch
	err = gitOperations.checkoutBranch()
	if err != nil {
		fmt.Println("error in `git checkout branch", gitOperations.Branch, "`, error: ", err)
	}

	// var sha1, sha2 [20]byte
	// copy(sha1[:], "6d18cfb9ea9cee75545f")
	// copy(sha2[:], "5e6b2616fa698ee6aa7e")

	// fmt.Println("SHA 1 & 2 ", sha1, "------", sha2)

	fileList, err := gitOperations.getChangedFilePaths(clonePath[1], "6d18cfb9ea9cee75545f0a02c86fa224f5347096", "445960eb841f2e18cc6aeb908de2e559f83ab1dd")
	if err != nil {
		fmt.Println("error in getting file list ", err)
	}
	fmt.Println("LIST OF FILES CHANGED ------ ", fileList)
	// Get the file changed between last and latest commit
	// r.client.Status().Update()
	// Define a new Pod object
	pod := newPodForCR(instance)

	// Set GitTrack instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, pod, r.scheme); err != nil {
		return reconcileGitTrack, err
	}

	// Check if this Pod already exists
	found := &corev1.Pod{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Pod", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
		err = r.client.Create(context.TODO(), pod)
		if err != nil {
			return reconcileGitTrack, err
		}
		return reconcileGitTrack, nil
	} else if err != nil {
		return reconcileGitTrack, err
	}

	// Pod already exists - don't requeue
	reqLogger.Info("Skip reconcile: Pod already exists", "Pod.Namespace", found.Namespace, "Pod.Name", found.Name)
	//os.RemoveAll(path.Join("/tmp", clonePath[1]))
	return reconcileGitTrack, nil
}

// newPodForCR returns a busybox pod with the same name/namespace as the cr
func newPodForCR(cr *mayadatav1alpha1.GitTrack) *corev1.Pod {
	labels := map[string]string{
		"app": cr.Name,
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-pod",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "busybox",
					Image:   "busybox",
					Command: []string{"sleep", "3600"},
				},
			},
		},
	}
}

func (r *ReconcileGitTrack) GetSecret(secretName, secretNamespace string) (*corev1.Secret, error) {
	found := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: secretNamespace}, found)
	if err != nil {
		return found, err
	}
	return found, nil
}
