import {QueryClient} from '@tanstack/query-core';
import {Clips, RecentEvents, Snapshots, Status} from '../wailsjs/go/main/App';

export const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            retry: false,
            staleTime: 0,
        },
    },
});

export const loadStatus = () => queryClient.fetchQuery({queryKey: ['status'], queryFn: Status});

export const loadRecentEvents = () =>
    queryClient.fetchQuery({queryKey: ['activity', 'events'], queryFn: RecentEvents});

export const loadRecordings = (collection: 'clips' | 'snapshots') =>
    queryClient.fetchQuery({
        queryKey: ['recordings', collection],
        queryFn: collection === 'clips' ? Clips : Snapshots,
    });
