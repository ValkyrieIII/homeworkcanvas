import { createRouter, createWebHistory } from "vue-router";
import HomeView from "./views/HomeView.vue";
import AssignmentsView from "./views/AssignmentsView.vue";
import ScheduleView from "./views/ScheduleView.vue";
import CoursesView from "./views/CoursesView.vue";
import CourseDetailLayout from "./views/CourseDetailLayout.vue";
import CourseAssignmentsView from "./views/CourseAssignmentsView.vue";
import CourseScheduleView from "./views/CourseScheduleView.vue";
import CourseSourcesView from "./views/CourseSourcesView.vue";
import CourseCandidatesView from "./views/CourseCandidatesView.vue";
import NotificationsView from "./views/NotificationsView.vue";
import CourseMembersView from "./views/CourseMembersView.vue";

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", component: HomeView },
    { path: "/assignments", component: AssignmentsView },
    { path: "/schedule", component: ScheduleView },
    { path: "/courses", component: CoursesView },
    { path: "/notifications", component: NotificationsView },
    {
      path: "/courses/:courseId",
      component: CourseDetailLayout,
      props: true,
      children: [
        { path: "", redirect: { name: "course-assignments" } },
        {
          path: "assignments",
          name: "course-assignments",
          component: CourseAssignmentsView,
          props: true,
        },
        {
          path: "schedule",
          name: "course-schedule",
          component: CourseScheduleView,
          props: true,
        },
        {
          path: "sources",
          name: "course-sources",
          component: CourseSourcesView,
          props: true,
        },
        {
          path: "candidates",
          name: "course-candidates",
          component: CourseCandidatesView,
          props: true,
        },
        {
          path: "members",
          name: "course-members",
          component: CourseMembersView,
          props: true,
        },
      ],
    },
    { path: "/:pathMatch(.*)*", redirect: "/" },
  ],
});
