import { createRouter, createWebHistory } from "vue-router";
import AssignmentsView from "./views/AssignmentsView.vue";
import UploadView from "./views/UploadView.vue";

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", redirect: "/assignments" },
    { path: "/assignments", component: AssignmentsView },
    { path: "/upload", component: UploadView },
    { path: "/:pathMatch(.*)*", redirect: "/assignments" },
  ],
});
