/**
 * Compile-time conformance check: every JSON in this folder is a REAL response of the
 * admin API (dumped from the Go handlers over a seeded database). Each one is assigned to
 * its interface in ../types.ts; tsc reports any missing, extra or mistyped field.
 *
 * Re-run:   cd web && node scripts/admin-types.test.mjs
 *      or:  cd web && node_modules/.bin/tsc --noEmit -p tsconfig.json
 *
 * String literal unions and boolean literals are widened (JSON imports only give `string`
 * and `boolean`); scripts/admin-types.test.mjs checks the literal values at runtime.
 */
import type {
  AdminCatalog,
  AdminCosts,
  AdminEnvelope,
  AdminErrorsSummary,
  AdminGrowth,
  AdminIssue,
  AdminIssueCreate,
  AdminIssueUpdate,
  AdminIssuesList,
  AdminMe,
  AdminOverview,
  AdminPlaybackErrors,
  AdminPlaybackHealth,
  AdminSources,
  AdminUserDetail,
  AdminUsersList,
  AdminUsersSummary,
  AdminViews,
} from "../types";

import me_session from "./me.session.json";
import me_token from "./me.token.json";
import overview from "./overview.json";
import overview_empty from "./overview.empty.json";
import views from "./views.json";
import views_empty from "./views.empty.json";
import catalog from "./catalog.json";
import catalog_movie from "./catalog.movie.json";
import catalog_empty from "./catalog.empty.json";
import users_summary from "./users-summary.json";
import users_summary_token from "./users-summary.token.json";
import users_session from "./users.session.json";
import users_token from "./users.token.json";
import user_detail_session from "./user-detail.session.json";
import user_detail_token from "./user-detail.token.json";
import user_detail_empty from "./user-detail.empty.session.json";
import growth from "./growth.json";
import growth_empty from "./growth.empty.json";
import playback_health from "./playback-health.json";
import playback_health_empty from "./playback-health.empty.json";
import playback_errors from "./playback-errors.json";
import playback_errors_summary from "./playback-errors-summary.json";
import playback_sources from "./playback-sources.json";
import costs from "./costs.json";
import costs_partial from "./costs.partial.json";
import costs_empty from "./costs.empty.json";
import issues from "./issues.json";
import issues_empty from "./issues.empty.json";
import issue from "./issue.json";
import issue_create_response from "./issue-create.response.json";
import issue_create_minimal_response from "./issue-create-minimal.response.json";
import issue_update_response from "./issue-update.response.json";
import issue_create_request from "./issue-create.request.json";
import issue_create_minimal_request from "./issue-create-minimal.request.json";
import issue_update_request from "./issue-update.request.json";

/** T with string literals and boolean literals widened to string / boolean. */
type Widen<T> = T extends string
  ? string
  : T extends boolean
    ? boolean
    : T extends readonly (infer E)[]
      ? Widen<E>[]
      : T extends object
        ? { [K in keyof T]: Widen<T[K]> }
        : T;

/** V with every key unknown to T turned into `never` (deep): detects extra fields. */
type NoExtra<V, T> = V extends readonly unknown[]
  ? Extract<T, readonly unknown[]> extends readonly (infer TE)[]
    ? NoExtra<V[number], TE>[]
    : never
  : V extends object
    ? Extract<T, object> extends infer TO
      ? [TO] extends [never]
        ? never
        : Exclude<keyof V, keyof TO> extends never
          ? { [K in keyof V]: NoExtra<V[K], K extends keyof TO ? TO[K] : never> }
          : never
      : never
    : V;

/** Fails to compile when the JSON value does not match T exactly. */
function exact<T>() {
  return <V extends Widen<T>>(value: V & NoExtra<V, Widen<T>>): void => {
    void value;
  };
}

type Env<T> = AdminEnvelope<T>;

exact<Env<AdminMe>>()(me_session);
exact<Env<AdminMe>>()(me_token);
exact<Env<AdminOverview>>()(overview);
exact<Env<AdminOverview>>()(overview_empty);
exact<Env<AdminViews>>()(views);
exact<Env<AdminViews>>()(views_empty);
exact<Env<AdminCatalog>>()(catalog);
exact<Env<AdminCatalog>>()(catalog_movie);
exact<Env<AdminCatalog>>()(catalog_empty);
exact<Env<AdminUsersSummary>>()(users_summary);
exact<Env<AdminUsersSummary>>()(users_summary_token);
exact<Env<AdminUsersList>>()(users_session);
exact<Env<AdminUsersList>>()(users_token);
exact<Env<AdminUserDetail>>()(user_detail_session);
exact<Env<AdminUserDetail>>()(user_detail_token);
exact<Env<AdminUserDetail>>()(user_detail_empty);
exact<Env<AdminGrowth>>()(growth);
exact<Env<AdminGrowth>>()(growth_empty);
exact<Env<AdminPlaybackHealth>>()(playback_health);
exact<Env<AdminPlaybackHealth>>()(playback_health_empty);
exact<Env<AdminPlaybackErrors>>()(playback_errors);
exact<Env<AdminErrorsSummary>>()(playback_errors_summary);
exact<Env<AdminSources>>()(playback_sources);
exact<Env<AdminCosts>>()(costs);
exact<Env<AdminCosts>>()(costs_partial);
exact<Env<AdminCosts>>()(costs_empty);
exact<Env<AdminIssuesList>>()(issues);
exact<Env<AdminIssuesList>>()(issues_empty);
exact<Env<AdminIssue>>()(issue);
exact<Env<AdminIssue>>()(issue_create_response);
exact<Env<AdminIssue>>()(issue_create_minimal_response);
exact<Env<AdminIssue>>()(issue_update_response);
exact<AdminIssueCreate>()(issue_create_request);
exact<AdminIssueCreate>()(issue_create_minimal_request);
exact<AdminIssueUpdate>()(issue_update_request);
