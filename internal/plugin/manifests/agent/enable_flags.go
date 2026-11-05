package agent

import (
	sharedcapture "github.com/netobserv/netobserv-operator/pkg/capture"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// Every Enable* method here is only ever called when its corresponding flag is true: bash's
// parse_args left the env var at its (already "false") template default whenever a flag was
// explicitly set to false, so there is no corresponding Disable* method to write.

// EnablePrivileged replaces edit_manifest "privileged". Also the side effect of --enable_pkt_drop,
// --enable_network_events, --enable_udn_mapping, --enable_openssl and --enable_all, matching bash.
func (b *Builder) EnablePrivileged() {
	c := b.container()
	if c.SecurityContext == nil {
		c.SecurityContext = &corev1.SecurityContext{}
	}
	c.SecurityContext.AllowPrivilegeEscalation = ptr.To(true)
	c.SecurityContext.Privileged = ptr.To(true)
}

// enableHostPID replaces edit_manifest "host_pid". Unexported: bash never exposes a standalone
// --host_pid flag, only --enable_openssl triggers it.
func (b *Builder) enableHostPID() {
	b.ds.Spec.Template.Spec.HostPID = true
}

// EnablePacketDrops replaces the --enable_pkt_drop branch of parse_args (privileged + env var).
func (b *Builder) EnablePacketDrops() {
	b.EnablePrivileged()
	b.enableFeature(sharedcapture.PacketDrop)
}

// EnableDNSTracking replaces the --enable_dns branch of parse_args.
func (b *Builder) EnableDNSTracking() {
	b.enableFeature(sharedcapture.DNSTracking)
}

// EnableRTT replaces the --enable_rtt branch of parse_args.
func (b *Builder) EnableRTT() {
	b.enableFeature(sharedcapture.FlowRTT)
}

// EnableNetworkEvents replaces the --enable_network_events branch of parse_args.
func (b *Builder) EnableNetworkEvents() {
	b.EnablePrivileged()
	b.enableFeature(sharedcapture.NetworkEvents)
}

// EnablePacketTranslation replaces the --enable_pkt_translation branch of parse_args.
func (b *Builder) EnablePacketTranslation() {
	b.enableFeature(sharedcapture.PacketTranslation)
}

// EnableIPSecTracking replaces the --enable_ipsec branch of parse_args.
func (b *Builder) EnableIPSecTracking() {
	b.enableFeature(sharedcapture.IPSec)
}

func (b *Builder) enableFeature(feature sharedcapture.Feature) {
	if name, ok := sharedcapture.FeatureEnvName(feature); ok {
		b.setEnv(name, "true")
	}
}

// EnableAll replaces the --enable_all branch of parse_args: every flows/metrics feature flag
// except OpenSSL plaintext capture (packets-only, never part of enable_all in bash either).
func (b *Builder) EnableAll() error {
	b.EnablePacketDrops()
	b.EnableDNSTracking()
	b.EnableRTT()
	b.EnableNetworkEvents()
	if err := b.EnableUDNMapping(); err != nil {
		return err
	}
	b.EnablePacketTranslation()
	b.EnableIPSecTracking()
	return nil
}

// EnableOpenSSLTracking replaces the --enable_openssl branch of parse_args (packets-only):
// privileged, hostPID, the host /usr,/lib,/lib64 mounts OpenSSL uprobe attachment needs, then
// the env var itself.
func (b *Builder) EnableOpenSSLTracking() {
	b.EnablePrivileged()
	b.enableHostPID()
	b.AddHostMountsForTLS()
	b.setEnv(envOpenSSLTracking, "true")
}

// AddHostMountsForTLS replaces edit_manifest "tls_host_mounts": host-usr/host-lib/host-lib64
// hostPath volumes/mounts, readOnlyRootFilesystem relaxed, and SYS_PTRACE added to the
// capability set. Each individual volume and mount is added at most once, so this remains
// safe when a partially configured pod is passed in.
func (b *Builder) AddHostMountsForTLS() {
	spec := &b.ds.Spec.Template.Spec
	c := b.container()
	for _, mount := range []struct {
		name string
		path string
	}{
		{name: "host-usr", path: "/usr"},
		{name: "host-lib", path: "/lib"},
		{name: "host-lib64", path: "/lib64"},
	} {
		if !hasVolume(spec.Volumes, mount.name) {
			spec.Volumes = append(spec.Volumes, hostPathVolume(mount.name, mount.path))
		}
		if !hasVolumeMount(c.VolumeMounts, mount.name) {
			c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
				Name: mount.name, MountPath: "/host" + mount.path, ReadOnly: true,
			})
		}
	}

	c = b.container()
	if c.SecurityContext == nil {
		c.SecurityContext = &corev1.SecurityContext{}
	}
	c.SecurityContext.ReadOnlyRootFilesystem = ptr.To(false)
	if c.SecurityContext.Capabilities == nil {
		c.SecurityContext.Capabilities = &corev1.Capabilities{}
	}
	if !hasCapability(c.SecurityContext.Capabilities.Add, "SYS_PTRACE") {
		c.SecurityContext.Capabilities.Add = append(c.SecurityContext.Capabilities.Add, "SYS_PTRACE")
	}
}

func hasVolume(volumes []corev1.Volume, name string) bool {
	for i := range volumes {
		if volumes[i].Name == name {
			return true
		}
	}
	return false
}

func hasVolumeMount(mounts []corev1.VolumeMount, name string) bool {
	for i := range mounts {
		if mounts[i].Name == name {
			return true
		}
	}
	return false
}

func hasCapability(caps []corev1.Capability, name corev1.Capability) bool {
	for _, c := range caps {
		if c == name {
			return true
		}
	}
	return false
}

func hostPathVolume(name, path string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: path,
				Type: ptr.To(corev1.HostPathDirectory),
			},
		},
	}
}
