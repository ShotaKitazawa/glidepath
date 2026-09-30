// Registered only so installability checks that still gate "Add to Home
// Screen" on an active service worker (older engines; current Chrome no
// longer requires one) see one. No caching/offline strategy — offline
// support and push notifications are both out of scope (CLAUDE.md).
self.addEventListener("fetch", () => {});
