/**
 * HyperFrames Render Service — HTTP server entry point.
 *
 * Exposes:
 *   GET  /health          — readiness probe
 *   POST /render          — sync MP4 render
 *   POST /lint            — lint project HTML (Phase 1: basic checks)
 *   POST /snapshot        — keyframe snapshots (Phase 2)
 *   POST /render/stream   — SSE progress stream (Phase 2)
 *   GET  /jobs/:jobId     — job status
 *
 * AIOS Go backend calls these instead of shelling out to npx hyperframes.
 */
declare const app: import("fastify").FastifyInstance<import("http").Server<typeof import("http").IncomingMessage, typeof import("http").ServerResponse>, import("http").IncomingMessage, import("http").ServerResponse<import("http").IncomingMessage>, import("fastify").FastifyBaseLogger, import("fastify").FastifyTypeProviderDefault> & PromiseLike<import("fastify").FastifyInstance<import("http").Server<typeof import("http").IncomingMessage, typeof import("http").ServerResponse>, import("http").IncomingMessage, import("http").ServerResponse<import("http").IncomingMessage>, import("fastify").FastifyBaseLogger, import("fastify").FastifyTypeProviderDefault>> & {
    __linterBrands: "SafePromiseLike";
};
export { app };
//# sourceMappingURL=server.d.ts.map