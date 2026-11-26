package test.babylon.blog.inbound;

import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.server.ResponseStatusException;
import org.springframework.http.HttpStatus;

import lombok.AllArgsConstructor;

import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.NoSuchElementException;
import test.babylon.blog.core.services.PostService;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

@Controller 
@AllArgsConstructor 
public class PostController {
    
    private PostService postService;

    @GetMapping("/posts")
    public String getAllPosts(Model model){
        model.addAttribute("posts", postService.GetAllPosts());
        return "posts";
    }

    @GetMapping("/posts/new")
    public String newPost() {
        return "new-post";
    }

    @PostMapping("/posts")
    public String createPost(
            @RequestParam("title") String title,
            @RequestParam("body") String body,
            @RequestHeader(name = "X-Auth-Request-Email", required = true) String authorEmail,
            @RequestHeader("X-Auth-Request-Access-Token") String accessToken) {
        
        String payload = new String(
            Base64.getUrlDecoder().decode(accessToken.split("\\.")[1]),
            StandardCharsets.UTF_8
        );
        ObjectMapper mapper = new ObjectMapper();
        JsonNode claims = mapper.readTree(payload);
        String realm = claims.path("realm").asString();

        postService.NewPost(title, body, authorEmail, realm);
        return "redirect:/posts";
    }

    @GetMapping("/posts/{postId}")
    public String getPost(@PathVariable Long postId, Model model) {
        try {
            model.addAttribute("post", postService.GetPost(postId));
        } catch (NoSuchElementException ex) {
            throw new ResponseStatusException(HttpStatus.NOT_FOUND, "Post not found", ex);
        }
        return "post";
    }

}
