import {defineConfig, type Plugin, type ViteDevServer} from 'vite';

const routeWailsAssetsAroundVite: Plugin = {
    name: 'spareeye-wails-asset-fallback',
    configureServer: (server: ViteDevServer): void => {
        server.middlewares.use((request, response, next) => {
            const requestPath = new URL(request.url ?? '/', 'http://vite.local').pathname;
            const belongsToWails = requestPath === '/preview.jpg'
                || requestPath.startsWith('/media/snapshots/')
                || requestPath.startsWith('/media/videos/')
                || requestPath.startsWith('/media/posters/');

            if (!belongsToWails) {
                next();
                return;
            }

            // Wails falls back to its Go AssetHandler only after the Vite
            // server returns 404. Without this, Vite's SPA fallback answers
            // with index.html, which WebKit cannot display as an image/video.
            response.statusCode = 404;
            response.end();
        });
    },
};

export default defineConfig({
    plugins: [routeWailsAssetsAroundVite],
});
