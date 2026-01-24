import { Router, Route } from "@solidjs/router";
import { lazy } from "solid-js";
import Header from "@/components/Header";
import Footer from "@/components/Footer";

const Landing = lazy(() => import("@/pages/Landing"));
const DocsLayout = lazy(() => import("@/pages/docs/DocsLayout"));
const Features = lazy(() => import("@/pages/docs/Features"));
const Settings = lazy(() => import("@/pages/docs/Settings"));
const Theme = lazy(() => import("@/pages/docs/Theme"));
const SelfHost = lazy(() => import("@/pages/docs/SelfHost"));

function Layout(props: { children?: any }) {
  return (
    <div class="flex min-h-screen flex-col">
      <Header />
      <main class="flex-1">{props.children}</main>
      <Footer />
    </div>
  );
}

function App() {
  return (
    <Router root={Layout}>
      <Route path="/" component={Landing} />
      <Route path="/docs" component={DocsLayout}>
        <Route path="/" component={Features} />
        <Route path="/features" component={Features} />
        <Route path="/settings" component={Settings} />
        <Route path="/theme" component={Theme} />
        <Route path="/self-host" component={SelfHost} />
      </Route>
    </Router>
  );
}

export default App;
